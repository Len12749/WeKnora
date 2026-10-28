package dingtalk

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	dtsdk "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/utils"

	"github.com/Tencent/WeKnora/internal/im"
	"github.com/Tencent/WeKnora/internal/logger"
)

// MessageHandler is called when an IM message is received via stream connection.
type MessageHandler func(ctx context.Context, msg *im.IncomingMessage) error

// LongConnClient manages a DingTalk Stream mode connection.
type LongConnClient struct {
	clientID     string
	clientSecret string
	handler      MessageHandler
	streamClient *dtsdk.StreamClient
}

// NewLongConnClient creates a DingTalk stream client.
func NewLongConnClient(clientID, clientSecret string, handler MessageHandler) *LongConnClient {
	cli := dtsdk.NewStreamClient(
		dtsdk.WithAppCredential(&dtsdk.AppCredentialConfig{
			ClientId:     clientID,
			ClientSecret: clientSecret,
		}),
	)

	c := &LongConnClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		handler:      handler,
		streamClient: cli,
	}

	// 自己注册原始帧处理器：SDK 的 BotCallbackDataModel 不含引用消息字段，
	// 只有解开原始 JSON 才能拿到 text.repliedMsg（/sub 需要它定位到被引用的那一轮）。
	cli.RegisterRouter(utils.SubscriptionTypeKCallback, payload.BotMessageCallbackTopic, c.onChatBotFrame)

	return c
}

// Start establishes the stream connection. The underlying SDK's Start is
// non-blocking: it dials the websocket, spawns its internal read/reconnect
// loops, and returns once the connection is established (or the attempt fails).
func (c *LongConnClient) Start(ctx context.Context) error {
	logger.Infof(ctx, "[IM] DingTalk Stream connecting...")
	return c.streamClient.Start(ctx)
}

// Close tears down the stream connection. AutoReconnect is disabled first so
// that closing the connection does not trigger the SDK's internal reconnect.
func (c *LongConnClient) Close() {
	c.streamClient.AutoReconnect = false
	c.streamClient.Close()
}

func (c *LongConnClient) onChatBotFrame(
	ctx context.Context,
	df *payload.DataFrame,
) (*payload.DataFrameResponse, error) {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(df.Data), &data); err != nil {
		return nil, err
	}

	incoming := streamToIncoming(&data, c.clientID)
	logDingTalkSubDiagnostic(ctx, df.Data)
	if quote := parseDingTalkQuote(df.Data); quote != nil {
		incoming.Quote = quote
		logger.Infof(ctx,
			"[IM] DingTalk quoted reply detected: msg_id=%s content_len=%d created_at=%s",
			quote.MessageID, len(quote.Content), formatQuoteTime(quote.CreatedAt))
	}

	if err := c.handler(ctx, incoming); err != nil {
		logger.Errorf(ctx, "[DingTalk] Handle message error: %v", err)
	}

	resp := payload.NewSuccessDataFrameResponse()
	resp.SetData("")
	return resp, nil
}

// logDingTalkSubDiagnostic 在收到 /sub 时打印引用相关字段，用于确认钉钉到底
// 以什么结构下发"被引用消息"。只打白名单字段，不落 sessionWebhook 这类敏感值。
func logDingTalkSubDiagnostic(ctx context.Context, raw string) {
	if !strings.Contains(strings.ToLower(raw), "/sub") {
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return
	}
	keep := []string{"msgtype", "text", "content", "originalMsgId", "originalProcessQueryKey"}
	parts := make([]string, 0, len(keep))
	for _, key := range keep {
		if value, ok := fields[key]; ok {
			text := string(value)
			if runes := []rune(text); len(runes) > 500 {
				text = string(runes[:500]) + "..."
			}
			parts = append(parts, key+"="+text)
		}
	}
	logger.Infof(ctx, "[IM] DingTalk /sub raw fields: %s", strings.Join(parts, " | "))
}

// parseDingTalkQuote 提取引用回复的目标消息。钉钉把被引用消息放在
// text.repliedMsg 里，SDK 未建模，只能按原始 JSON 读。
func parseDingTalkQuote(raw string) *im.QuotedMessage {
	var body struct {
		Text struct {
			RepliedMsg struct {
				MsgID     string          `json:"msgId"`
				SenderID  string          `json:"senderId"`
				CreatedAt int64           `json:"createdAt"`
				MsgType   string          `json:"msgType"`
				Content   json.RawMessage `json:"content"`
			} `json:"repliedMsg"`
		} `json:"text"`
		Content struct {
			Quoted struct {
				MsgID        string `json:"msgId"`
				CreatedAt    int64  `json:"createdAt"`
				CardCreated  int64  `json:"cardCreatedAt"`
				IsQuotedCard bool   `json:"isQuotedCard"`
			} `json:"quoted"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil
	}

	msgID, content, createdAt := "", "", int64(0)
	if replied := body.Text.RepliedMsg; replied.MsgID != "" || len(replied.Content) > 0 || replied.CreatedAt > 0 {
		msgID, createdAt = replied.MsgID, replied.CreatedAt
		content = extractQuotedText(replied.Content)
	} else if quoted := body.Content.Quoted; quoted.MsgID != "" || quoted.CreatedAt > 0 || quoted.CardCreated > 0 {
		msgID = quoted.MsgID
		switch {
		case quoted.CreatedAt > 0:
			createdAt = quoted.CreatedAt
		case quoted.CardCreated > 0:
			createdAt = quoted.CardCreated
		}
	}

	if msgID == "" && content == "" && createdAt == 0 {
		return nil
	}
	quote := &im.QuotedMessage{MessageID: msgID, Content: content}
	if createdAt > 0 {
		quote.CreatedAt = time.UnixMilli(createdAt)
	}
	return quote
}

// extractQuotedText 从被引用消息的 content 里抠出可读文本。
// 文本消息是 JSON 字符串；机器人回答是 interactiveCard，正文散落在
// cardContent[].children[].value 里，这里按 value 递归收一遍。
func extractQuotedText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asMap interface{}
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return ""
	}
	var texts []string
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch node := v.(type) {
		case map[string]interface{}:
			if value, ok := node["value"].(string); ok && strings.TrimSpace(value) != "" {
				texts = append(texts, value)
			}
			if value, ok := node["content"].(string); ok && strings.TrimSpace(value) != "" {
				texts = append(texts, value)
			}
			for key, child := range node {
				if key == "value" || key == "content" {
					continue
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(asMap)
	joined := strings.TrimSpace(strings.Join(texts, ""))
	if runes := []rune(joined); len(runes) > 200 {
		joined = string(runes[:200])
	}
	return joined
}

func formatQuoteTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
