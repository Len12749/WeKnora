package im

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newSubTestDB 建一张最小 messages 表：/sub 只需要 role、content 与时间序。
func newSubTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := "file:im-sub-" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(name), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.Exec(`CREATE TABLE messages (
		id varchar(36) PRIMARY KEY,
		session_id varchar(36),
		role varchar(32),
		content text,
		created_at datetime,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}
	return db
}

func insertMessage(t *testing.T, db *gorm.DB, id, sessionID, role, content string, at time.Time) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, sessionID, role, content, at,
	).Error
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func TestLatestQAPair_ReturnsLastTurn(t *testing.T) {
	db := newSubTestDB(t)
	base := time.Now().Add(-time.Hour)
	insertMessage(t, db, "m1", "s1", "user", "第一问", base)
	insertMessage(t, db, "m2", "s1", "assistant", "第一答", base.Add(time.Minute))
	insertMessage(t, db, "m3", "s1", "user", "第二问", base.Add(2*time.Minute))
	insertMessage(t, db, "m4", "s1", "assistant", "第二答", base.Add(3*time.Minute))
	// 另一个会话的更新消息不应被选中
	insertMessage(t, db, "m5", "s2", "user", "别的会话", base.Add(4*time.Minute))

	svc := &Service{db: db}
	question, answer, err := svc.latestQAPair(context.Background(), "s1")
	if err != nil {
		t.Fatalf("latestQAPair: %v", err)
	}
	if question != "第二问" || answer != "第二答" {
		t.Fatalf("取到的是 %q / %q，期望 第二问 / 第二答", question, answer)
	}
}

func TestLatestQAPair_QuestionOnlyWhenNoAnswer(t *testing.T) {
	db := newSubTestDB(t)
	insertMessage(t, db, "m1", "s1", "user", "刚问完还在跑", time.Now())

	svc := &Service{db: db}
	question, answer, err := svc.latestQAPair(context.Background(), "s1")
	if err != nil {
		t.Fatalf("latestQAPair: %v", err)
	}
	if question != "刚问完还在跑" || answer != "" {
		t.Fatalf("取到的是 %q / %q，期望只锁问题", question, answer)
	}
}

func TestLatestQAPair_EmptySession(t *testing.T) {
	db := newSubTestDB(t)
	svc := &Service{db: db}
	question, answer, err := svc.latestQAPair(context.Background(), "missing")
	if err != nil {
		t.Fatalf("latestQAPair: %v", err)
	}
	if question != "" || answer != "" {
		t.Fatalf("空会话应返回空，实际 %q / %q", question, answer)
	}
}
