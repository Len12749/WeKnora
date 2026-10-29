package im

import (
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/logger"
)

// imMessageDedup 是"这条 IM 消息已经处理过"的落库记录。
// 表结构见 migrations/sqlite/000054_im_message_dedup.up.sql。
type imMessageDedup struct {
	MessageID string    `gorm:"column:message_id;type:varchar(128);primaryKey"`
	Platform  string    `gorm:"column:platform;type:varchar(32)"`
	ChannelID string    `gorm:"column:channel_id;type:varchar(36)"`
	CreatedAt time.Time `gorm:"column:created_at;index"`
}

func (imMessageDedup) TableName() string { return "im_message_dedup" }

// claimMessage 抢占一条消息的处理权，返回 false 表示它已经被处理过。
//
// 去重状态必须跨进程存活：飞书/钉钉在 ACK 超时、长连接重连后会把同一条消息
// 重新投递，实测同一条消息隔 2 小时后还会再来一次；只放进程内 map 时，
// 重启一次去重表就清零了（19:02 那次重复回复就是这么漏进来的）。
//
// 优先级：Redis（多实例）→ DB（单实例也持久）→ 进程内 map（DB 不可用时兜底，
// 宁可极端情况下重复回复，也不静默吞掉用户消息）。
func (s *Service) claimMessage(ctx context.Context, messageID, platform, channelID string) bool {
	if s.redis != nil {
		key := RedisKeyDedup + messageID
		ok, err := s.redis.SetNX(ctx, key, "1", dedupTTL).Result()
		if err == nil {
			return ok
		}
		// Redis 配置了却失败：宁可丢这条（用户可以重发），也不要跨实例重复回答。
		logger.Errorf(ctx, "[IM] Redis dedup failed (fail-closed, message dropped): %v", err)
		return false
	}

	if s.db != nil {
		rec := &imMessageDedup{
			MessageID: messageID,
			Platform:  platform,
			ChannelID: channelID,
			CreatedAt: time.Now(),
		}
		res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec)
		if res.Error == nil {
			return res.RowsAffected > 0
		}
		// 写库失败不能挡用户：退化到进程内去重，至少挡住同一进程内的重投。
		logger.Errorf(ctx, "[IM] DB dedup failed, falling back to in-memory: %v", res.Error)
	}

	_, loaded := s.processedMsgs.LoadOrStore(messageID, time.Now())
	return !loaded
}

// purgeClaimedMessages 清理过期的去重记录，避免表无限增长。
func (s *Service) purgeClaimedMessages(ctx context.Context, cutoff time.Time) {
	if s.db == nil {
		return
	}
	res := s.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&imMessageDedup{})
	if res.Error != nil {
		logger.Warnf(ctx, "[IM] purge dedup records failed: %v", res.Error)
	}
}
