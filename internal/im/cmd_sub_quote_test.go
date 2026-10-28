package im

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newQuoteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:sub-quote-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { if s, err := db.DB(); err == nil { _ = s.Close() } })
	if err := db.Exec(`CREATE TABLE messages (
		id varchar(36) PRIMARY KEY, session_id varchar(36), role varchar(32),
		content text, created_at datetime, deleted_at datetime)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	return db
}

func addMsg(t *testing.T, db *gorm.DB, id, role, content string, at time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO messages (id,session_id,role,content,created_at) VALUES (?,?,?,?,?)`,
		id, "s1", role, content, at).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestResolveTicketTarget_ByQuoteTime(t *testing.T) {
	db := newQuoteTestDB(t)
	base := time.Now().Add(-30 * time.Minute)
	addMsg(t, db, "m1", "user", "第一问", base)
	addMsg(t, db, "m2", "assistant", "第一答", base.Add(5*time.Second))
	addMsg(t, db, "m3", "user", "第二问", base.Add(10*time.Minute))
	addMsg(t, db, "m4", "assistant", "第二答", base.Add(10*time.Minute+5*time.Second))

	svc := &Service{db: db}
	// 引用「第一答」（钉钉给的发送时间）
	quote := &QuotedMessage{CreatedAt: base.Add(6 * time.Second), IsBotMessage: true}
	q, a, err := svc.resolveTicketTarget(context.Background(), "s1", quote)
	if err != nil {
		t.Fatal(err)
	}
	if q != "第一问" || a != "第一答" {
		t.Fatalf("按引用时间应锁第一轮，实际 %q / %q", q, a)
	}
}

func TestResolveTicketTarget_ByQuoteContent(t *testing.T) {
	db := newQuoteTestDB(t)
	base := time.Now().Add(-30 * time.Minute)
	addMsg(t, db, "m1", "user", "包赔能转手继承吗", base)
	addMsg(t, db, "m2", "assistant", "**包赔**挂在订单上，不能继承。", base.Add(5*time.Second))
	addMsg(t, db, "m3", "user", "提现多久到账", base.Add(10*time.Minute))
	addMsg(t, db, "m4", "assistant", "T+1 到账。", base.Add(10*time.Minute+5*time.Second))

	svc := &Service{db: db}
	quote := &QuotedMessage{Content: "包赔挂在订单上", IsBotMessage: true}
	q, a, err := svc.resolveTicketTarget(context.Background(), "s1", quote)
	if err != nil {
		t.Fatal(err)
	}
	if q != "包赔能转手继承吗" || a != "**包赔**挂在订单上，不能继承。" {
		t.Fatalf("按引用正文应锁第一轮，实际 %q / %q", q, a)
	}
}

func TestResolveTicketTarget_QuoteFromOtherUserFallsBackToLatest(t *testing.T) {
	db := newQuoteTestDB(t)
	base := time.Now().Add(-30 * time.Minute)
	addMsg(t, db, "m1", "user", "第一问", base)
	addMsg(t, db, "m2", "assistant", "第一答", base.Add(5*time.Second))
	addMsg(t, db, "m3", "user", "第二问", base.Add(10*time.Minute))
	addMsg(t, db, "m4", "assistant", "第二答", base.Add(10*time.Minute+5*time.Second))

	svc := &Service{db: db}
	// 引用的是别人的消息：即使时间能对上第一轮，也必须按"最近一轮"提交
	quote := &QuotedMessage{CreatedAt: base.Add(6 * time.Second), IsBotMessage: false}
	q, a, err := svc.resolveTicketTarget(context.Background(), "s1", quote)
	if err != nil {
		t.Fatal(err)
	}
	if q != "第二问" || a != "第二答" {
		t.Fatalf("引用非机器人消息应回落到最近一轮，实际 %q / %q", q, a)
	}
}

func TestResolveTicketTarget_NoQuoteFallsBackToLatest(t *testing.T) {
	db := newQuoteTestDB(t)
	base := time.Now().Add(-30 * time.Minute)
	addMsg(t, db, "m1", "user", "第一问", base)
	addMsg(t, db, "m2", "assistant", "第一答", base.Add(5*time.Second))
	addMsg(t, db, "m3", "user", "第二问", base.Add(10*time.Minute))
	addMsg(t, db, "m4", "assistant", "第二答", base.Add(10*time.Minute+5*time.Second))

	svc := &Service{db: db}
	q, a, err := svc.resolveTicketTarget(context.Background(), "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if q != "第二问" || a != "第二答" {
		t.Fatalf("无引用应锁最近一轮，实际 %q / %q", q, a)
	}
}
