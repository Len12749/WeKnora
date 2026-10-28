package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// newTicketTestService 用真实的 sqlite 迁移脚本建表，确保 GORM 字段名与表结构一致。
func newTicketTestService(t *testing.T) (*ticketService, *gorm.DB) {
	t.Helper()
	name := "file:ticket-svc-" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(name), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	sqlBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "sqlite", "000052_ticket.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if err := db.Exec(string(sqlBytes)).Error; err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return &ticketService{db: db}, db
}

func TestTicketServiceLifecycle(t *testing.T) {
	svc, _ := newTicketTestService(t)
	ctx := context.Background()

	created := &types.Ticket{
		ID:            "t-1",
		TenantID:      1,
		ChannelID:     "ch-1",
		Platform:      "dingtalk",
		SessionID:     "s-1",
		Question:      "包赔能继承吗",
		Answer:        "不能，包赔挂在订单上。",
		SubmitterID:   "im-user-1",
		SubmitterName: "刘森",
		Status:        types.TicketStatusPending,
	}
	if err := svc.Create(ctx, created); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, total, err := svc.List(ctx, types.TicketListQuery{TenantID: 1, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("list 返回 total=%d len=%d", total, len(list))
	}

	// 关键词命中问题文本
	_, total, err = svc.List(ctx, types.TicketListQuery{TenantID: 1, Keyword: "包赔", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list by keyword: %v", err)
	}
	if total != 1 {
		t.Fatalf("关键词检索应命中 1 条，实际 %d", total)
	}

	note := "已确认为产品口径问题"
	updated, err := svc.UpdateStatus(ctx, 1, "t-1", types.TicketStatusResolved, &note, "u-admin", "管理员")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if updated.Status != types.TicketStatusResolved {
		t.Fatalf("状态未更新：%s", updated.Status)
	}
	if updated.Note != note {
		t.Fatalf("备注未更新：%s", updated.Note)
	}
	if updated.StatusUpdatedAt == nil || updated.StatusUpdatedBy != "u-admin" || updated.StatusUpdatedByName != "管理员" {
		t.Fatalf("状态操作人记录不完整：%+v", updated.StatusUpdatedAt)
	}
	if time.Since(*updated.StatusUpdatedAt) > time.Minute {
		t.Fatalf("状态更新时间不合理：%v", updated.StatusUpdatedAt)
	}

	// 单独改备注不能覆盖状态操作人
	note2 := "补充说明"
	refreshed, err := svc.UpdateNote(ctx, 1, "t-1", note2)
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	if refreshed.Note != note2 || refreshed.StatusUpdatedBy != "u-admin" {
		t.Fatalf("备注更新影响了状态操作人：note=%s by=%s", refreshed.Note, refreshed.StatusUpdatedBy)
	}

	// 跨租户不可见
	if _, err := svc.Get(ctx, 2, "t-1"); err == nil {
		t.Fatal("跨租户读取不应成功")
	}

	deleted, err := svc.Delete(ctx, 1, []string{"t-1"})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("删除条数=%d", deleted)
	}
	if _, total, _ = svc.List(ctx, types.TicketListQuery{TenantID: 1, Page: 1, PageSize: 20}); total != 0 {
		t.Fatalf("软删除后列表应清空，实际 %d", total)
	}
}

func TestTicketServiceRejectsBadStatus(t *testing.T) {
	svc, _ := newTicketTestService(t)
	if _, err := svc.UpdateStatus(context.Background(), 1, "missing", "unknown", nil, "", ""); err == nil {
		t.Fatal("非法状态应报错")
	}
}
