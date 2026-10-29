package im

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newDedupTestDB 用真实的 sqlite 迁移脚本建去重表，保证 GORM 字段与表结构一致。
func newDedupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := "file:im-dedup-" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(name), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	sqlBytes, err := os.ReadFile(filepath.Join("..", "..", "migrations", "sqlite", "000054_im_message_dedup.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if err := db.Exec(string(sqlBytes)).Error; err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return db
}

// TestClaimMessageSurvivesRestart 覆盖真实事故：飞书把同一条消息在 2 小时后重投，
// 此时进程已经重启过。去重状态在库里，所以第二次必须先被拦下。
func TestClaimMessageSurvivesRestart(t *testing.T) {
	db := newDedupTestDB(t)
	ctx := context.Background()

	first := &Service{db: db}
	if !first.claimMessage(ctx, "om_msg_1", "feishu", "ch-1") {
		t.Fatal("first delivery must be accepted")
	}

	// 模拟进程重启：新的 Service 实例，进程内 map 是空的。
	restarted := &Service{db: db}
	if restarted.claimMessage(ctx, "om_msg_1", "feishu", "ch-1") {
		t.Fatal("redelivery after restart must be rejected")
	}

	// 不同消息互不影响。
	if !restarted.claimMessage(ctx, "om_msg_2", "feishu", "ch-1") {
		t.Fatal("another message must be accepted")
	}
}

// TestClaimMessageWithoutDBFallsBackToMemory DB 不可用时不能挡住用户消息。
func TestClaimMessageWithoutDBFallsBackToMemory(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	if !svc.claimMessage(ctx, "om_msg_3", "feishu", "ch-1") {
		t.Fatal("first delivery must be accepted without DB")
	}
	if svc.claimMessage(ctx, "om_msg_3", "feishu", "ch-1") {
		t.Fatal("in-process redelivery must be rejected without DB")
	}
}

// TestPurgeClaimedMessages 只清过期记录，24 小时窗口内的重投仍要被拦。
func TestPurgeClaimedMessages(t *testing.T) {
	db := newDedupTestDB(t)
	ctx := context.Background()
	svc := &Service{db: db}

	if !svc.claimMessage(ctx, "om_old", "feishu", "ch-1") {
		t.Fatal("first delivery must be accepted")
	}
	// 把记录改到 TTL 之外。
	if err := db.Exec("UPDATE im_message_dedup SET created_at = ? WHERE message_id = ?",
		time.Now().Add(-2*dedupTTL), "om_old").Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	svc.purgeClaimedMessages(ctx, time.Now().Add(-dedupTTL))

	var count int64
	if err := db.Model(&imMessageDedup{}).Where("message_id = ?", "om_old").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expired record should be purged, got %d", count)
	}
}
