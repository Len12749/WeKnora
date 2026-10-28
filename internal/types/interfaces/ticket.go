package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// TicketService 管理 IM 反馈工单的存取与状态流转。
//
// 工单是"用户看到一次不满意的回答后提交的快照"，因此没有编辑正文的入口：
// 只允许改状态、写备注、删除。
type TicketService interface {
	// Create 落一条新工单；调用方负责填好快照与提交人。
	Create(ctx context.Context, ticket *types.Ticket) error
	// Get 按租户取单条，不存在或跨租户返回错误。
	Get(ctx context.Context, tenantID uint64, id string) (*types.Ticket, error)
	// List 返回当前页数据与总数。
	List(ctx context.Context, query types.TicketListQuery) ([]*types.Ticket, int64, error)
	// UpdateStatus 改状态并记录操作人；note 非 nil 时同事务更新备注。
	UpdateStatus(
		ctx context.Context,
		tenantID uint64,
		id string,
		status types.TicketStatus,
		note *string,
		actorID, actorName string,
	) (*types.Ticket, error)
	// UpdateNote 单独更新备注（不改状态、不覆盖状态操作人）。
	UpdateNote(ctx context.Context, tenantID uint64, id, note string) (*types.Ticket, error)
	// Delete 软删除一批工单，返回实际删除条数。
	Delete(ctx context.Context, tenantID uint64, ids []string) (int64, error)
}
