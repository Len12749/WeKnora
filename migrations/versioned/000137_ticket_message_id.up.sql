-- Migration: 000055_ticket_message_id（PostgreSQL 变体）
-- 同一条 IM 消息重复投递时只落一张工单；历史数据允许为空。
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS message_id VARCHAR(128) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uk_tickets_message_id
    ON tickets (message_id) WHERE message_id <> '';
