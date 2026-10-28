package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// TicketHandler 暴露 IM 反馈工单的查询与状态维护接口。
//
// 权限边界：读接口由路由层用 Viewer 兜住（同空间成员都能看到工单），
// 改状态 / 删除 / 批量删除用 Admin 兜住。
type TicketHandler struct {
	tickets interfaces.TicketService
	users   interfaces.UserService
}

// NewTicketHandler 构造工单 handler。
func NewTicketHandler(tickets interfaces.TicketService, users interfaces.UserService) *TicketHandler {
	return &TicketHandler{tickets: tickets, users: users}
}

// ListTickets 分页返回当前空间的工单，支持关键词与状态筛选。
func (h *TicketHandler) ListTickets(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("缺少空间上下文"))
		return
	}

	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)

	var status types.TicketStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" && raw != "all" {
		parsed, valid := types.ParseTicketStatus(raw)
		if !valid {
			c.Error(apperrors.NewBadRequestError("非法工单状态"))
			return
		}
		status = parsed
	}

	items, total, err := h.tickets.List(ctx, types.TicketListQuery{
		TenantID: tenantID,
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetTicket 返回单条工单详情（含备注）。
func (h *TicketHandler) GetTicket(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("缺少空间上下文"))
		return
	}
	ticket, err := h.tickets.Get(ctx, tenantID, c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ticket})
}

// ticketUpdateRequest 允许一次请求里同时改状态与备注。
type ticketUpdateRequest struct {
	Status *string `json:"status"`
	Note   *string `json:"note"`
}

// UpdateTicket 修改工单状态（可选同时写备注）。仅管理员可调用。
func (h *TicketHandler) UpdateTicket(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("缺少空间上下文"))
		return
	}
	if !h.isAdmin(ctx) {
		c.Error(apperrors.NewForbiddenError("仅管理员可修改工单"))
		return
	}

	var req ticketUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}
	if req.Status == nil && req.Note == nil {
		c.Error(apperrors.NewBadRequestError("没有需要更新的内容"))
		return
	}

	id := c.Param("id")
	var (
		ticket *types.Ticket
		err    error
	)

	if req.Status == nil {
		ticket, err = h.tickets.UpdateNote(ctx, tenantID, id, *req.Note)
	} else {
		status, valid := types.ParseTicketStatus(*req.Status)
		if !valid {
			c.Error(apperrors.NewBadRequestError("非法工单状态"))
			return
		}
		actorID, _ := types.UserIDFromContext(ctx)
		ticket, err = h.tickets.UpdateStatus(ctx, tenantID, id, status, req.Note, actorID, h.actorName(ctx, actorID))
	}
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ticket})
}

// DeleteTicket 软删除单条工单。仅管理员可调用。
func (h *TicketHandler) DeleteTicket(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("缺少空间上下文"))
		return
	}
	if !h.isAdmin(ctx) {
		c.Error(apperrors.NewForbiddenError("仅管理员可删除工单"))
		return
	}
	deleted, err := h.tickets.Delete(ctx, tenantID, []string{c.Param("id")})
	if err != nil {
		c.Error(err)
		return
	}
	if deleted == 0 {
		c.Error(apperrors.NewNotFoundError("工单不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": deleted}})
}

// batchDeleteTicketsRequest 是批量删除的请求体。
type batchDeleteTicketsRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

// BatchDeleteTickets 批量软删除工单。仅管理员可调用。
func (h *TicketHandler) BatchDeleteTickets(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("缺少空间上下文"))
		return
	}
	if !h.isAdmin(ctx) {
		c.Error(apperrors.NewForbiddenError("仅管理员可删除工单"))
		return
	}
	var req batchDeleteTicketsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}
	// 单次批量上限，避免一次请求扫全表。
	if len(req.IDs) > 200 {
		c.Error(apperrors.NewBadRequestError("单次最多删除 200 条"))
		return
	}
	deleted, err := h.tickets.Delete(ctx, tenantID, req.IDs)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": deleted}})
}

// isAdmin 自己判一次管理员身份：路由层的 Admin guard 在 EnableRBAC=false 时
// 只记日志不拦截，而工单的写操作按需求必须对普通成员关闭。
func (h *TicketHandler) isAdmin(ctx context.Context) bool {
	if types.IsSystemAdminFromContext(ctx) {
		return true
	}
	return types.CallerFromContext(ctx).Role.HasPermission(types.TenantRoleAdmin)
}

// actorName 把用户 id 解析成展示名；查不到时返回空串，前端回退显示 id。
func (h *TicketHandler) actorName(ctx context.Context, userID string) string {
	if h.users == nil || userID == "" {
		return ""
	}
	user, err := h.users.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return ""
	}
	return user.Username
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return fallback
	}
	return v
}
