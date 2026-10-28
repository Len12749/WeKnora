-- Migration: 000053_ticket_submit_reason（PostgreSQL 变体）
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS submit_reason TEXT;
