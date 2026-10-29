-- Migration: 000055_ticket_message_id
-- 记录触发 /sub 的那条 IM 消息 id，并作为幂等键：同一条消息无论被投递几次，
-- 都只落一张工单。历史数据允许为空，所以用部分唯一索引放开空值。
ALTER TABLE tickets ADD COLUMN message_id varchar(128) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uk_tickets_message_id
    ON tickets (message_id) WHERE message_id <> '';
