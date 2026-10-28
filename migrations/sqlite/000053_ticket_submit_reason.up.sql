-- Migration: 000053_ticket_submit_reason
-- /sub 可以带提交理由（如「知识库里没有陪玩，但是它回复有」），单独存一列，
-- 与管理员备注 note 分开：理由由提交人写，备注由管理员写。
ALTER TABLE tickets ADD COLUMN submit_reason text;
