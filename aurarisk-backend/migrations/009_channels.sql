-- SMS and USSD access through Africa's Talking (#61).
--
-- Each phone number gets an account so channel reports use the same
-- duplicate detection and reporter reputation as app reports. Only a salted
-- hash of the number is stored (Data Protection Act 2019). Deleting the
-- account removes the mapping.
CREATE TABLE IF NOT EXISTS channel_reporters (
	phone_hash BYTEA PRIMARY KEY,
	account_id UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Where a report came from. SMS and USSD reports are placed at the county
-- headquarters, so their location is approximate: they don't corroborate
-- other reports, and clients can show them as county-level.
ALTER TABLE community_reports
	ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'app'
		CHECK (source IN ('app', 'sms', 'ussd')),
	ADD COLUMN IF NOT EXISTS location_approximate BOOLEAN NOT NULL DEFAULT FALSE;
