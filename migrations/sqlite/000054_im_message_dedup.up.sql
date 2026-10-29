-- Migration: 000054_im_message_dedup
-- IM 消息去重表：飞书在 ACK 超时或长连接重连后会把同一条消息重新投递，
-- 实测同一条消息隔 2 小时再次到达。去重状态必须落库，进程重启才不会丢。
CREATE TABLE IF NOT EXISTS im_message_dedup (
    message_id varchar(128) NOT NULL PRIMARY KEY,
    platform   varchar(32)  NOT NULL DEFAULT '',
    channel_id varchar(36)  NOT NULL DEFAULT '',
    created_at datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_im_message_dedup_created ON im_message_dedup (created_at);
