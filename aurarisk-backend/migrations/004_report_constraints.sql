-- Enforce report validation in the database too. NOT VALID skips checking
-- rows written before these constraints existed but applies to new rows.
ALTER TABLE community_reports
	DROP CONSTRAINT IF EXISTS community_reports_lat_check,
	DROP CONSTRAINT IF EXISTS community_reports_lon_check,
	DROP CONSTRAINT IF EXISTS community_reports_category_check;
ALTER TABLE community_reports
	ADD CONSTRAINT community_reports_lat_check CHECK (lat BETWEEN -90 AND 90) NOT VALID,
	ADD CONSTRAINT community_reports_lon_check CHECK (lon BETWEEN -180 AND 180) NOT VALID,
	ADD CONSTRAINT community_reports_category_check
		CHECK (category IN ('flooding', 'road_blocked', 'water_rising', 'drainage_issue')) NOT VALID;
