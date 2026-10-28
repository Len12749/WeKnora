package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/im"
	"github.com/Tencent/WeKnora/internal/logger"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// fetchedMessage 是 GET /open-apis/im/v1/messages/:id 里的一条消息。
type fetchedMessage struct {
	MessageID  string `json:"message_id"`
	CreateTime string `json:"create_time"` // 毫秒时间戳，字符串
	MsgType    string `json:"msg_type"`
	Body       struct {
		Content string `json:"content"`
	} `json:"body"`
	Sender struct {
		ID         string `json:"id"`
		SenderType string `json:"sender_type"` // "app" 表示应用（机器人）发的
	} `json:"sender"`
}

type fetchedMessagesResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Items []fetchedMessage `json:"items"`
	} `json:"data"`
}

type contactUserResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	} `json:"data"`
}

// feishuGet 发一个带 tenant token 的 GET，并把 JSON 解到 out。
func (a *Adapter) feishuGet(ctx context.Context, token, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu api %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return json.Unmarshal(body, out)
}

// fetchQuoteForReply 用 parent_id 拉被引用的那条消息，转成统一的 QuotedMessage。
//
// 飞书的事件里只给 parent_id，不给内容，所以必须回查一次消息详情：
// create_time 用于 /sub 按时间定位那一轮，body.content 用作文本兜底。
// 拉不到就返回 nil（调用方按"没有引用"处理），不要让 /sub 因此失败。
func (a *Adapter) fetchQuoteForReply(ctx context.Context, parentID string) *im.QuotedMessage {
	if parentID == "" {
		return nil
	}
	token, err := a.getTenantAccessToken(ctx)
	if err != nil {
		logger.Warnf(ctx, "[%s] quoted reply: get tenant token failed: %v", a.region.Label, err)
		return nil
	}

	var resp fetchedMessagesResponse
	endpoint := a.api("/open-apis/im/v1/messages/%s", parentID)
	if err := a.feishuGet(ctx, token, endpoint, &resp); err != nil {
		logger.Warnf(ctx, "[%s] quoted reply: fetch message %s failed: %v", a.region.Label, parentID, err)
		return nil
	}
	if resp.Code != 0 {
		logger.Warnf(ctx, "[%s] quoted reply: fetch message %s rejected: code=%d msg=%s",
			a.region.Label, parentID, resp.Code, resp.Msg)
		return nil
	}
	if len(resp.Data.Items) == 0 {
		return nil
	}

	msg := resp.Data.Items[0]
	quote := &im.QuotedMessage{
		MessageID: msg.MessageID,
		Content:   extractFeishuQuoteText(msg),
		SenderID:  msg.Sender.ID,
		// 只有应用（机器人）发的消息才算"引用机器人回复"。
		IsBotMessage: msg.Sender.SenderType == "app",
	}
	if ts, err := strconv.ParseInt(msg.CreateTime, 10, 64); err == nil && ts > 0 {
		quote.CreatedAt = time.UnixMilli(ts)
	}
	if quote.Content == "" && quote.CreatedAt.IsZero() {
		return nil
	}
	return quote
}

// extractFeishuQuoteText 从消息体里抠出可读文本；非文本类型只返回类型名。
func extractFeishuQuoteText(msg fetchedMessage) string {
	switch msg.MsgType {
	case "text":
		var body struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(msg.Body.Content), &body); err == nil {
			return strings.TrimSpace(body.Text)
		}
	case "post":
		var body struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag  string `json:"tag"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(msg.Body.Content), &body); err == nil {
			parts := make([]string, 0, 4)
			if body.Title != "" {
				parts = append(parts, body.Title)
			}
			for _, line := range body.Content {
				var lineText strings.Builder
				for _, elem := range line {
					if elem.Tag == "text" || elem.Tag == "a" {
						lineText.WriteString(elem.Text)
					}
				}
				if t := strings.TrimSpace(lineText.String()); t != "" {
					parts = append(parts, t)
				}
			}
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
	}
	return ""
}

// senderNameEntry 缓存 open_id → 名字，避免每条消息都打一次通讯录接口。
type senderNameEntry struct {
	name string
	at   time.Time
}

var (
	senderNameCache sync.Map // key: appID|openID
	senderNameTTL   = 30 * time.Minute
)

// resolveSenderName 把飞书 open_id 解析成可读名字。
//
// 失败时返回空串（工单里就显示 open_id），不影响消息处理。常见的失败原因是
// 应用没有开 contact:user.base:readonly 权限，日志里会看到 code=99991672。
func (a *Adapter) resolveSenderName(ctx context.Context, openID string) string {
	if openID == "" {
		return ""
	}
	key := a.appID + "|" + openID
	if cached, ok := senderNameCache.Load(key); ok {
		entry := cached.(senderNameEntry)
		if time.Since(entry.at) < senderNameTTL {
			return entry.name
		}
	}

	token, err := a.getTenantAccessToken(ctx)
	if err != nil {
		senderNameCache.Store(key, senderNameEntry{at: time.Now()})
		return ""
	}

	var resp contactUserResponse
	endpoint := a.api("/open-apis/contact/v3/users/%s?user_id_type=open_id", openID)
	if err := a.feishuGet(ctx, token, endpoint, &resp); err != nil {
		logger.Warnf(ctx, "[%s] resolve sender name failed: %v", a.region.Label, err)
		senderNameCache.Store(key, senderNameEntry{at: time.Now()})
		return ""
	}
	if resp.Code != 0 {
		logger.Warnf(ctx,
			"[%s] resolve sender name rejected: code=%d msg=%s（需要 contact:user.base:readonly 权限）",
			a.region.Label, resp.Code, resp.Msg)
		senderNameCache.Store(key, senderNameEntry{at: time.Now()})
		return ""
	}

	name := strings.TrimSpace(resp.Data.User.Name)
	senderNameCache.Store(key, senderNameEntry{name: name, at: time.Now()})
	return name
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

// enrichIncoming 给长连接路径的消息补上引用信息与发送人昵称。
//
// 飞书 WebSocket 事件和 webhook 事件是两套解析入口（convertEvent vs ParseCallback），
// 这里统一挂在长连接分支上，保证两条路都拿到同样的补全。
func (a *Adapter) enrichIncoming(
	ctx context.Context,
	msg *im.IncomingMessage,
	event *larkim.P2MessageReceiveV1,
) *im.IncomingMessage {
	if msg == nil || event == nil || event.Event == nil {
		return msg
	}
	if event.Event.Message == nil {
		return msg
	}

	parentID := ""
	if event.Event.Message.ParentId != nil {
		parentID = *event.Event.Message.ParentId
	}
	openID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil && event.Event.Sender.SenderId.OpenId != nil {
		openID = *event.Event.Sender.SenderId.OpenId
	}

	if msg.UserName == "" {
		msg.UserName = a.resolveSenderName(ctx, openID)
	}
	if parentID != "" {
		quote := a.fetchQuoteForReply(ctx, parentID)
		msg.Quote = quote
		logger.Infof(ctx,
			"[%s] reply event: msg_id=%s parent_id=%s quote_ok=%v quote_from_bot=%v",
			a.region.Label, msg.MessageID, parentID, quote != nil, quote != nil && quote.IsBotMessage)
	}
	return msg
}
