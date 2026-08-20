-- Send's authorization relaxed from self-or-platform.admin to any
-- authenticated caller (driven by a real product need: Pulse's Phase 5
-- push fallback, the first real cross-user notification sender this
-- platform has ever had - see notifications/README.md). sent_by_user_id
-- keeps every cross-user send properly attributed for audit, since the
-- old self-or-admin restriction made the recipient (user_id) and the
-- actor implicitly the same person (or an admin) - that's no longer
-- true, so it can no longer be left implicit.
ALTER TABLE notification_requests ADD COLUMN IF NOT EXISTS sent_by_user_id UUID REFERENCES users(id);

-- Backfill existing rows: they were all self-sent (the only case
-- possible under the old rule, since admin-sent rows are rare/absent
-- historically and indistinguishable after the fact) - sent_by_user_id
-- defaults to the recipient for pre-existing data.
UPDATE notification_requests SET sent_by_user_id = user_id WHERE sent_by_user_id IS NULL;
