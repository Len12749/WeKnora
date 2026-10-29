-- Migration: 000054_im_message_dedup（PostgreSQL 变体）
-- IM 消息去重表：平台在 ACK 超时或长连接重连后会把同一条消息重新投递，
-- 去重状态必须落库，进程重启才不会丢。
CREATE TABLE IF NOT EXISTS im_message_dedup (
    message_id VARCHAR(128) NOT NULL PRIMARY KEY,
    platform   VARCHAR(32)  NOT NULL DEFAULT '',
    channel_id VARCHAR(36)  NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_im_message_dedup_created ON im_message_dedup (created_at);
