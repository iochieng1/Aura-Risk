CREATE TABLE IF NOT EXISTS community_reports (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	location_name TEXT NOT NULL,
	lat DECIMAL(10, 6) NOT NULL,
	lon DECIMAL(10, 6) NOT NULL,
	category TEXT NOT NULL,
	note TEXT NOT NULL,
	created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reports_location ON community_reports(lat, lon);
