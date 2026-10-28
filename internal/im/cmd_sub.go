package im

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// TicketCommand 实现 /sub：把当前会话里最近一轮问答存成一条反馈工单。
//
// 定位规则（对应用户"锁这一次的对话"的诉求）：
//   - 支持引用回复的平台上，引用哪条都行，命令本身不依赖引用内容；
//   - 一律锁会话里"最近一条 assistant 回答 + 它前面那条 user 问题"；
//   - 还没有回答时（刚问完、还在跑），退化成只锁最近一条问题。
//
// 历史消息不参与，工单只保存提交时刻的快照。
type TicketCommand struct {
	svc *Service
}

func newTicketCommand(svc *Service) *TicketCommand {
	return &TicketCommand{svc: svc}
}

func (c *TicketCommand) Name() string { return "sub" }

func (c *TicketCommand) Description() string {
	return "提交工单：可回复某条回答只锁那一轮，也可附上理由，例如：`/sub 知识库里没有陪玩`"
}

func (c *TicketCommand) Execute(
	ctx context.Context,
	cmdCtx *CommandContext,
	args []string,
) (*CommandResult, error) {
	if cmdCtx == nil || cmdCtx.Session == nil || cmdCtx.Incoming == nil {
		return &CommandResult{Content: "当前会话不可用，无法提交工单。"}, nil
	}
	if c.svc == nil || c.svc.ticketService == nil {
		return &CommandResult{Content: "工单服务未启用，请联系管理员。"}, nil
	}

	reason := strings.TrimSpace(strings.Join(args, " "))
	question, answer, err := c.svc.resolveTicketTarget(ctx, cmdCtx.Session.SessionID, cmdCtx.Incoming.Quote)
	if err != nil {
		logger.Warnf(ctx, "[IM] /sub 读取会话消息失败: session=%s err=%v", cmdCtx.Session.SessionID, err)
		return &CommandResult{Content: "读取会话记录失败，请稍后重试。"}, nil
	}
	if strings.TrimSpace(question) == "" && strings.TrimSpace(answer) == "" {
		return &CommandResult{Content: "这个会话里还没有可提交的问答，先问一个问题再发 `/sub`。"}, nil
	}

	ticket := &types.Ticket{
		ID:            uuid.New().String(),
		TenantID:      cmdCtx.TenantID,
		ChannelID:     cmdCtx.Session.IMChannelID,
		Platform:      string(cmdCtx.Incoming.Platform),
		ChatID:        cmdCtx.Incoming.ChatID,
		SessionID:     cmdCtx.Session.SessionID,
		Question:      question,
		Answer:        answer,
		SubmitterID:   cmdCtx.Incoming.UserID,
		SubmitterName: cmdCtx.Incoming.UserName,
		SubmitReason:  reason,
		Status:        types.TicketStatusPending,
	}
	if err := c.svc.ticketService.Create(ctx, ticket); err != nil {
		logger.Warnf(ctx, "[IM] /sub 创建工单失败: session=%s err=%v", cmdCtx.Session.SessionID, err)
		return &CommandResult{Content: "工单提交失败，请稍后重试。"}, nil
	}

	logger.Infof(ctx, "[IM] /sub 已创建工单: id=%s session=%s user=%s",
		ticket.ID, ticket.SessionID, ticket.SubmitterID)

	// 提交成功只需要一句确认，细节都在 Web 端看。
	return &CommandResult{Content: "已提交工单，感谢您对知识库优化做出的贡献。"}, nil
}

// resolveTicketTarget 决定 /sub 要锁哪一轮问答。
//
// 优先用引用消息定位：钉钉会给被引用消息的发送时间（有时还带正文），
// 按时间找最近的那条 assistant 消息，比"最近一轮"精确得多。
// 拿不到引用信息时退回到最近一轮。
func (s *Service) resolveTicketTarget(
	ctx context.Context,
	sessionID string,
	quote *QuotedMessage,
) (string, string, error) {
	// 只有"引用机器人的回复"才用于定位那一轮；引用别人的消息、或压根没引用，
	// 都按最近一轮提交，避免把别人的问答错配进来。
	if quote == nil || !quote.IsBotMessage || (quote.CreatedAt.IsZero() && strings.TrimSpace(quote.Content) == "") {
		return s.latestQAPair(ctx, sessionID)
	}

	messages, err := s.recentMessagesAsc(ctx, sessionID, 50)
	if err != nil {
		return "", "", err
	}

	if !quote.CreatedAt.IsZero() {
		best, bestDiff := -1, 5*time.Minute
		for i, msg := range messages {
			if msg.Role != "assistant" {
				continue
			}
			diff := msg.CreatedAt.Sub(quote.CreatedAt)
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				best, bestDiff = i, diff
			}
		}
		if best >= 0 {
			question, answer := pairAround(messages, best)
			if answer != "" {
				return question, answer, nil
			}
		}
	}

	if content := normalizeQuoteText(quote.Content); content != "" {
		for i, msg := range messages {
			if msg.Role == "assistant" && strings.Contains(normalizeQuoteText(msg.Content), content) {
				question, answer := pairAround(messages, i)
				if answer != "" {
					return question, answer, nil
				}
			}
		}
	}

	return s.latestQAPair(ctx, sessionID)
}

// recentMessagesAsc 取最近 limit 条消息并按时间升序返回，便于向前找问题。
func (s *Service) recentMessagesAsc(ctx context.Context, sessionID string, limit int) ([]types.Message, error) {
	if s.db == nil || sessionID == "" {
		return nil, nil
	}
	var messages []types.Message
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// pairAround 从第 idx 条 assistant 消息往上找它对应的问题。
func pairAround(messages []types.Message, idx int) (string, string) {
	if idx < 0 || idx >= len(messages) {
		return "", ""
	}
	answer := messages[idx].Content
	for i := idx - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content, answer
		}
	}
	return "", answer
}

// normalizeQuoteText 把引用正文压成可比对的短串：去空白、去 markdown 记号。
func normalizeQuoteText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '*', '`', '#', '>', '-', '|', '\n', '\r', '\t', ' ':
			continue
		default:
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) > 40 {
		runes = runes[:40]
	}
	return string(runes)
}

// latestQAPair 取会话里最近一对「问题 + 回答」。
// 消息按时间倒序取一页，够覆盖"回答被截断重生成"之类的重复条目。
func (s *Service) latestQAPair(ctx context.Context, sessionID string) (string, string, error) {
	if s.db == nil || sessionID == "" {
		return "", "", nil
	}

	var messages []types.Message
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Limit(20).
		Find(&messages).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", err
	}

	var question, answer string
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch {
		case answer == "" && msg.Role == "assistant":
			answer = msg.Content
		case answer != "" && question == "" && msg.Role == "user":
			question = msg.Content
		}
		if question != "" && answer != "" {
			break
		}
	}

	// 只有问题没有回答：仍在生成或生成失败，只锁问题。
	if question == "" {
		for _, msg := range messages {
			if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
				question = msg.Content
				break
			}
		}
	}
	return question, answer, nil
}

// shortTicketID 让回复里只出现 uuid 前 8 位，够用户在列表里对上号。
func shortTicketID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}
