-- Kenyan county and ward on reports (#59), so they can be routed to and
-- counted for the county government responsible. Both are optional: older
-- reports and reports from elsewhere have neither. The API validates the
-- county against the 47 counties; ward is free text.
ALTER TABLE community_reports
	ADD COLUMN IF NOT EXISTS county TEXT,
	ADD COLUMN IF NOT EXISTS ward TEXT;
ALTER TABLE community_reports
	DROP CONSTRAINT IF EXISTS community_reports_ward_check;
ALTER TABLE community_reports
	ADD CONSTRAINT community_reports_ward_check
		CHECK (ward IS NULL OR (county IS NOT NULL AND octet_length(ward) BETWEEN 1 AND 100));
CREATE INDEX IF NOT EXISTS idx_reports_public_county ON community_reports(county, created_at DESC)
	WHERE county IS NOT NULL AND duplicate_of IS NULL AND moderation_status <> 'rejected';
