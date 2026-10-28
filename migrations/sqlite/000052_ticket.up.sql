-- Migration: 000052_ticket
-- IM 侧 /sub 提交的反馈工单：固化「问题 + 回答」快照，供 Web 端标记处理状态。
-- 回答正文可能很长，question/answer/note 都用 TEXT，不做长度上限。

CREATE TABLE IF NOT EXISTS tickets (
    id                     varchar(36)  NOT NULL PRIMARY KEY,
    tenant_id              integer      NOT NULL DEFAULT 0,
    channel_id             varchar(36)  NOT NULL DEFAULT '',
    platform               varchar(32)  NOT NULL DEFAULT '',
    chat_id                varchar(128) NOT NULL DEFAULT '',
    session_id             varchar(36)  NOT NULL DEFAULT '',
    question               text,
    answer                 text,
    submitter_id           varchar(128) NOT NULL DEFAULT '',
    submitter_name         varchar(128) NOT NULL DEFAULT '',
    status                 varchar(32)  NOT NULL DEFAULT 'pending',
    status_updated_at      datetime,
    status_updated_by      varchar(36)  NOT NULL DEFAULT '',
    status_updated_by_name varchar(128) NOT NULL DEFAULT '',
    note                   text,
    created_at             datetime,
    updated_at             datetime,
    deleted_at             datetime
);

CREATE INDEX IF NOT EXISTS idx_tickets_tenant_created ON tickets (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets (status);
CREATE INDEX IF NOT EXISTS idx_tickets_session ON tickets (session_id);
CREATE INDEX IF NOT EXISTS idx_tickets_submitter ON tickets (submitter_id);
CREATE INDEX IF NOT EXISTS idx_tickets_deleted_at ON tickets (deleted_at);
