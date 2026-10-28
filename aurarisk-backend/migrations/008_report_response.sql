-- County response status (#62), separate from moderation: open ->
-- acknowledged -> resolved, set by the county's responders. Timestamps feed
-- the public county dashboard (#63); events are the audit trail.
ALTER TABLE community_reports
	ADD COLUMN IF NOT EXISTS response_status TEXT NOT NULL DEFAULT 'open'
		CHECK (response_status IN ('open', 'acknowledged', 'resolved')),
	ADD COLUMN IF NOT EXISTS response_updated_at TIMESTAMPTZ,
	ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ,
	ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS report_response_events (
	id BIGSERIAL PRIMARY KEY,
	report_id UUID NOT NULL REFERENCES community_reports(id) ON DELETE CASCADE,
	from_status TEXT NOT NULL,
	to_status TEXT NOT NULL,
	actor TEXT NOT NULL,
	note TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_response_events_report ON report_response_events(report_id);

-- Responders list their county's verified reports by response status.
CREATE INDEX IF NOT EXISTS idx_reports_county_response ON community_reports(county, response_status, created_at DESC)
	WHERE county IS NOT NULL AND duplicate_of IS NULL AND moderation_status = 'verified';

-- Outbox for the per-county webhook fired when a report is verified. One row
-- per report; the worker retries with backoff until delivered or given up.
CREATE TABLE IF NOT EXISTS report_webhook_deliveries (
	report_id UUID PRIMARY KEY REFERENCES community_reports(id) ON DELETE CASCADE,
	county TEXT NOT NULL,
	attempts INT NOT NULL DEFAULT 0,
	next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	delivered_at TIMESTAMPTZ,
	last_error TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_due ON report_webhook_deliveries(next_attempt_at)
	WHERE delivered_at IS NULL;
