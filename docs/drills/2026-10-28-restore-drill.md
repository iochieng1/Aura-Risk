# Restore drill 2026-10-28

**Rehearsal on synthetic data, not production.** This was the first run of
`deploy/scripts/restore-drill.sh`. The source was a local PostGIS database
migrated to version 8 and seeded with 100,000 synthetic reports and 5,000
accounts, which is more than the live demo holds. It shows the procedure works
end to end and gives a rough size for timings. The release gate in #27 still
needs the same drill run against a backup of the production database once
production hosting is chosen (see [RELEASE.md](../RELEASE.md#backups-and-restore-drills)).

- **Result:** PASS
- **When:** 2026-10-28 08:06 UTC
- **Source:** postgres://***@localhost:5434/drill_source?sslmode=disable
- **Backup:** custom-format `pg_dump`, 6.7M; dump took 6s
- **Restore:** into a scratch `postgis/postgis:16-3.5-alpine` container, 9s (`pg_restore -j 4`)
- **Total time:** 43s
- **Schema version:** source 8, restored 8; PostGIS 3.5.7
- **Radius query on restored data** (50 km of Kisumu): 15940 report(s)

| Table | Source rows | Restored rows | |
|---|---|---|---|
| `community_reports` | 100000 | 100000 | ok |
| `accounts` | 5000 | 5000 | ok |
| `devices` | 0 | 0 | ok |
| `location_subscriptions` | 0 | 0 | ok |
| `report_photos` | 0 | 0 | ok |
| `report_moderation_events` | 0 | 0 | ok |
| `report_response_events` | 0 | 0 | ok |
| `schema_migrations` | 8 | 8 | ok |
