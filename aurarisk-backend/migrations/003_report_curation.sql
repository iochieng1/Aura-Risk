-- Report curation: moderation status, duplicate detection, automatic
-- verification scores, an audit trail, and indexes for retention cleanup.
-- Existing reports start as pending, which stays publicly visible.
ALTER TABLE community_reports
	ADD COLUMN IF NOT EXISTS moderation_status TEXT NOT NULL DEFAULT 'pending'
		CHECK (moderation_status IN ('pending', 'verified', 'rejected')),
	ADD COLUMN IF NOT EXISTS duplicate_of UUID REFERENCES community_reports(id) ON DELETE CASCADE,
	ADD COLUMN IF NOT EXISTS verification_score SMALLINT NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS corroboration_count INT NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS moderated_by TEXT,
	ADD COLUMN IF NOT EXISTS moderation_reason TEXT,
	ADD COLUMN IF NOT EXISTS status_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_reports_status_created ON community_reports(moderation_status, created_at);
CREATE INDEX IF NOT EXISTS idx_reports_created ON community_reports(created_at);
CREATE INDEX IF NOT EXISTS idx_reports_account ON community_reports(account_id) WHERE account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_reports_duplicate_of ON community_reports(duplicate_of) WHERE duplicate_of IS NOT NULL;

CREATE TABLE IF NOT EXISTS report_moderation_events (
	id BIGSERIAL PRIMARY KEY,
	report_id UUID NOT NULL REFERENCES community_reports(id) ON DELETE CASCADE,
	from_status TEXT NOT NULL,
	to_status TEXT NOT NULL,
	actor TEXT NOT NULL,
	reason TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_moderation_events_report ON report_moderation_events(report_id);

CREATE INDEX IF NOT EXISTS idx_report_photos_status_updated ON report_photos(status, updated_at);
