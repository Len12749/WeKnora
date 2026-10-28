package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterTicketRoutes 注册 IM 反馈工单的接口。
//
// 读面（列表、详情）对同空间所有成员开放，工单是"大家都能看到问题反馈"的存在；
// 写面（改状态、备注、删除、批量删除）限 Admin 及以上。
func RegisterTicketRoutes(r *gin.RouterGroup, ticketHandler *handler.TicketHandler, g *rbacGuards) {
	if ticketHandler == nil {
		return
	}
	tickets := g.apiKeyGroup(r.Group("/tickets", g.Viewer()), apiKeyFullAccess())
	{
		tickets.GET("", ticketHandler.ListTickets)
		tickets.GET("/:id", ticketHandler.GetTicket)
		tickets.PATCH("/:id", g.Admin(), ticketHandler.UpdateTicket)
		tickets.DELETE("/:id", g.Admin(), ticketHandler.DeleteTicket)
		tickets.POST("/batch-delete", g.Admin(), ticketHandler.BatchDeleteTickets)
	}
}
