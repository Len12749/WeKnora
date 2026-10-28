-- Migration: 000052_ticket
-- IM 侧 /sub 提交的反馈工单。与 migrations/sqlite/000052_ticket 对应：
-- 快照字段只增不改，删工单走 deleted_at 软删除。

CREATE TABLE IF NOT EXISTS tickets (
    id                     VARCHAR(36)  NOT NULL PRIMARY KEY,
    tenant_id              BIGINT       NOT NULL DEFAULT 0,
    channel_id             VARCHAR(36)  NOT NULL DEFAULT '',
    platform               VARCHAR(32)  NOT NULL DEFAULT '',
    chat_id                VARCHAR(128) NOT NULL DEFAULT '',
    session_id             VARCHAR(36)  NOT NULL DEFAULT '',
    question               TEXT,
    answer                 TEXT,
    submitter_id           VARCHAR(128) NOT NULL DEFAULT '',
    submitter_name         VARCHAR(128) NOT NULL DEFAULT '',
    status                 VARCHAR(32)  NOT NULL DEFAULT 'pending',
    status_updated_at      TIMESTAMP WITH TIME ZONE,
    status_updated_by      VARCHAR(36)  NOT NULL DEFAULT '',
    status_updated_by_name VARCHAR(128) NOT NULL DEFAULT '',
    note                   TEXT,
    created_at             TIMESTAMP WITH TIME ZONE,
    updated_at             TIMESTAMP WITH TIME ZONE,
    deleted_at             TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_tickets_tenant_created ON tickets (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets (status);
CREATE INDEX IF NOT EXISTS idx_tickets_session ON tickets (session_id);
CREATE INDEX IF NOT EXISTS idx_tickets_submitter ON tickets (submitter_id);
CREATE INDEX IF NOT EXISTS idx_tickets_deleted_at ON tickets (deleted_at);
