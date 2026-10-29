DROP INDEX IF EXISTS uk_tickets_message_id;
ALTER TABLE tickets DROP COLUMN IF EXISTS message_id;
