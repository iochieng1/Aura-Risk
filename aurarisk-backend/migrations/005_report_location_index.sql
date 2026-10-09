-- The public map query (GET /api/reports) only reads non-duplicate,
-- non-rejected reports inside a lat/lon box.
CREATE INDEX IF NOT EXISTS idx_reports_public_location ON community_reports(lat, lon)
	WHERE duplicate_of IS NULL AND moderation_status <> 'rejected';
