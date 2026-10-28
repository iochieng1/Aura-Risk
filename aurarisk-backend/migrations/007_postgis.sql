-- PostGIS radius searches for reports (#18). A generated geography column
-- mirrors lat/lon, so writers and the API keep using lat/lon, and a GiST
-- index serves ST_DWithin lookups (public listing, duplicate detection and
-- corroboration). Neon, RDS and DigitalOcean Postgres all ship PostGIS.
--
-- To reverse: DROP INDEX idx_reports_geog, idx_reports_public_geog;
-- ALTER TABLE community_reports DROP COLUMN geog; then recreate the lat/lon
-- indexes from 001 and 005. The extension can stay.
CREATE EXTENSION IF NOT EXISTS postgis;

ALTER TABLE community_reports
	ADD COLUMN IF NOT EXISTS geog geography(Point, 4326)
		GENERATED ALWAYS AS (ST_SetSRID(ST_MakePoint(lon::float8, lat::float8), 4326)::geography) STORED;

CREATE INDEX IF NOT EXISTS idx_reports_geog ON community_reports USING GIST (geog);
-- The public map query only reads non-duplicate, non-rejected reports.
CREATE INDEX IF NOT EXISTS idx_reports_public_geog ON community_reports USING GIST (geog)
	WHERE duplicate_of IS NULL AND moderation_status <> 'rejected';

-- Replaced by the spatial indexes above.
DROP INDEX IF EXISTS idx_reports_location;
DROP INDEX IF EXISTS idx_reports_public_location;
