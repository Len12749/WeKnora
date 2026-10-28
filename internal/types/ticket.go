package types

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// TicketStatus 是工单的处理状态。
type TicketStatus string

const (
	// TicketStatusPending 刚由 IM 提交，尚未有人认领。
	TicketStatusPending TicketStatus = "pending"
	// TicketStatusInProgress 已有人开始处理。
	TicketStatusInProgress TicketStatus = "in_progress"
	// TicketStatusResolved 已处理完。
	TicketStatusResolved TicketStatus = "resolved"
)

// ParseTicketStatus 校验前端传入的状态值。
func ParseTicketStatus(raw string) (TicketStatus, bool) {
	switch TicketStatus(strings.TrimSpace(raw)) {
	case TicketStatusPending:
		return TicketStatusPending, true
	case TicketStatusInProgress:
		return TicketStatusInProgress, true
	case TicketStatusResolved:
		return TicketStatusResolved, true
	default:
		return "", false
	}
}

// Ticket 是 IM 侧 /sub 提交的一条反馈工单。
//
// 它只固化「那一轮的问题 + 回答」的快照，不跟随会话后续变化：
// 回答被重新生成、会话被删除，都不影响已经提交的工单。
type Ticket struct {
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// TenantID 决定工单归属于哪个空间，列表与状态变更都按它隔离。
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// ChannelID / Platform / ChatID 记录来源通道，便于追查是哪个群里提的。
	ChannelID string `json:"channel_id" gorm:"type:varchar(36)"`
	Platform  string `json:"platform" gorm:"type:varchar(32)"`
	ChatID    string `json:"chat_id" gorm:"type:varchar(128)"`
	// SessionID 是 WeKnora 会话 id，用于回溯原始对话。
	SessionID string `json:"session_id" gorm:"type:varchar(36);index"`
	// Question / Answer 是提交时刻的快照原文。
	Question string `json:"question" gorm:"type:text"`
	Answer   string `json:"answer" gorm:"type:text"`
	// SubmitterID 是 IM 平台侧的用户标识（不是 WeKnora 账号 id）。
	SubmitterID   string `json:"submitter_id" gorm:"type:varchar(128);index"`
	SubmitterName string `json:"submitter_name" gorm:"type:varchar(128)"`
	// SubmitReason 是提交时 /sub 后面跟的那句话，说明为什么觉得回答有问题。
	SubmitReason string `json:"submit_reason" gorm:"type:text"`

	Status TicketStatus `json:"status" gorm:"type:varchar(32);index;default:pending"`
	// StatusUpdatedAt / By / ByName 记录"上一次改状态"的时间与人。
	StatusUpdatedAt     *time.Time `json:"status_updated_at"`
	StatusUpdatedBy     string     `json:"status_updated_by" gorm:"type:varchar(36)"`
	StatusUpdatedByName string     `json:"status_updated_by_name" gorm:"type:varchar(128)"`

	// Note 是管理员备注，只在详情里展示。
	Note string `json:"note" gorm:"type:text"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 固定表名，避免 GORM 复数化规则变更。
func (Ticket) TableName() string { return "tickets" }

// TicketListQuery 是工单列表的筛选条件。
type TicketListQuery struct {
	TenantID uint64
	// Keyword 同时匹配问题、回答与提交人。
	Keyword string
	// Status 为空表示不限状态。
	Status   TicketStatus
	Page     int
	PageSize int
}
