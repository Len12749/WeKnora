package service

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ticketService 是 TicketService 的 GORM 实现。
//
// 工单量级很小（几十人团队、每天个位数），所以直接读写 DB，不再拆 repository 层。
type ticketService struct {
	db *gorm.DB
}

// NewTicketService 构造工单服务。
func NewTicketService(db *gorm.DB) interfaces.TicketService {
	return &ticketService{db: db}
}

func (s *ticketService) Create(ctx context.Context, ticket *types.Ticket) error {
	if ticket == nil {
		return apperrors.NewBadRequestError("工单内容为空")
	}
	if ticket.Status == "" {
		ticket.Status = types.TicketStatusPending
	}
	return s.db.WithContext(ctx).Create(ticket).Error
}

func (s *ticketService) Get(ctx context.Context, tenantID uint64, id string) (*types.Ticket, error) {
	var ticket types.Ticket
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&ticket).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewNotFoundError("工单不存在")
		}
		return nil, err
	}
	return &ticket, nil
}

func (s *ticketService) List(
	ctx context.Context,
	query types.TicketListQuery,
) ([]*types.Ticket, int64, error) {
	page, pageSize := normalizeTicketPaging(query.Page, query.PageSize)

	db := s.db.WithContext(ctx).Model(&types.Ticket{}).Where("tenant_id = ?", query.TenantID)
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	// 关键词同时打问题、回答与提交人：运营多半记得"是谁提的"或"问的哪句话"。
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where(
			"question LIKE ? OR answer LIKE ? OR submitter_name LIKE ? OR submitter_id LIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	tickets := make([]*types.Ticket, 0, pageSize)
	err := db.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&tickets).Error
	if err != nil {
		return nil, 0, err
	}
	return tickets, total, nil
}

func (s *ticketService) UpdateStatus(
	ctx context.Context,
	tenantID uint64,
	id string,
	status types.TicketStatus,
	note *string,
	actorID, actorName string,
) (*types.Ticket, error) {
	if _, ok := types.ParseTicketStatus(string(status)); !ok {
		return nil, apperrors.NewBadRequestError("非法工单状态")
	}

	updates := map[string]interface{}{
		"status":                 status,
		"status_updated_at":      time.Now(),
		"status_updated_by":      actorID,
		"status_updated_by_name": actorName,
	}
	if note != nil {
		updates["note"] = *note
	}

	result := s.db.WithContext(ctx).
		Model(&types.Ticket{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.NewNotFoundError("工单不存在")
	}
	return s.Get(ctx, tenantID, id)
}

func (s *ticketService) UpdateNote(
	ctx context.Context,
	tenantID uint64,
	id, note string,
) (*types.Ticket, error) {
	result := s.db.WithContext(ctx).
		Model(&types.Ticket{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("note", note)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.NewNotFoundError("工单不存在")
	}
	return s.Get(ctx, tenantID, id)
}

func (s *ticketService) Delete(ctx context.Context, tenantID uint64, ids []string) (int64, error) {
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return 0, apperrors.NewBadRequestError("未指定工单")
	}
	result := s.db.WithContext(ctx).
		Where("tenant_id = ? AND id IN ?", tenantID, clean).
		Delete(&types.Ticket{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// normalizeTicketPaging 收敛分页参数，避免越界查询。
func normalizeTicketPaging(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
