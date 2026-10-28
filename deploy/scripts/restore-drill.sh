#!/usr/bin/env bash
# Backup and restore drill (#27): restores a backup into a scratch PostGIS
# container, checks it against the source, and prints a Markdown record to
# keep in docs/drills/. Needs Docker; pg_dump runs inside the container, so
# no local Postgres tools are required.
#
#   SOURCE_DATABASE_URL=postgres://... deploy/scripts/restore-drill.sh            # dump, then restore
#   DUMP_FILE=backups/aurarisk-....dump SOURCE_DATABASE_URL=... restore-drill.sh  # restore an existing backup
#
# SOURCE_DATABASE_URL is only read (pg_dump and row counts). For a database on
# this machine, use host.docker.internal (or the docker bridge IP) as the host.
set -euo pipefail

: "${SOURCE_DATABASE_URL:?set SOURCE_DATABASE_URL (read only)}"
image="${DRILL_IMAGE:-postgis/postgis:16-3.5-alpine}"
name="aurarisk-restore-drill-$$"
work="$(mktemp -d)"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

now() { date +%s; }
psql_src() { docker run --rm --network host "$image" psql "$SOURCE_DATABASE_URL" -XAtq -c "$1"; }
psql_dst() { docker exec "$name" psql -U postgres -d restored -XAtq -c "$1"; }

start=$(now)
if [ -n "${DUMP_FILE:-}" ]; then
  cp "$DUMP_FILE" "$work/backup.dump"
  dump_seconds="n/a (existing backup $(basename "$DUMP_FILE"))"
else
  docker run --rm --network host -v "$work:/backup" "$image" \
    pg_dump --format=custom --no-owner --no-privileges --file=/backup/backup.dump "$SOURCE_DATABASE_URL"
  dump_seconds=$(( $(now) - start ))
fi
size=$(du -h "$work/backup.dump" | cut -f1)

docker run -d --name "$name" -e POSTGRES_PASSWORD=drill "$image" >/dev/null
# The image starts a temporary server to initialise (and install PostGIS),
# then restarts: wait for the second "ready".
until [ "$(docker logs "$name" 2>&1 | grep -c 'ready to accept connections')" -ge 2 ]; do sleep 1; done
# From template0, so the image's preinstalled extensions don't clash with the
# ones in the backup.
docker exec "$name" createdb -U postgres -T template0 restored

restore_start=$(now)
docker cp "$work/backup.dump" "$name:/tmp/backup.dump"
docker exec "$name" pg_restore -U postgres -d restored --no-owner --no-privileges --exit-on-error -j 4 /tmp/backup.dump
restore_seconds=$(( $(now) - restore_start ))

# Checks: schema version, PostGIS, and per-table row counts against the source.
failures=0
src_version=$(psql_src "SELECT MAX(version) FROM schema_migrations")
dst_version=$(psql_dst "SELECT MAX(version) FROM schema_migrations")
postgis=$(psql_dst "SELECT extversion FROM pg_extension WHERE extname = 'postgis'")
[ "$src_version" = "$dst_version" ] || failures=$((failures + 1))
[ -n "$postgis" ] || failures=$((failures + 1))

rows="| Table | Source rows | Restored rows | |\n|---|---|---|---|\n"
for table in community_reports accounts devices location_subscriptions report_photos \
  report_moderation_events report_response_events channel_reporters schema_migrations; do
  exists=$(psql_dst "SELECT to_regclass('public.$table') IS NOT NULL")
  [ "$exists" = "t" ] || continue
  src=$(psql_src "SELECT COUNT(*) FROM $table")
  dst=$(psql_dst "SELECT COUNT(*) FROM $table")
  mark="ok"
  if [ "$src" != "$dst" ]; then mark="**differs**"; failures=$((failures + 1)); fi
  rows+="| \`$table\` | $src | $dst | $mark |\n"
done
# The spatial index answers a radius query on the restored data.
spatial=$(psql_dst "SELECT COUNT(*) FROM community_reports WHERE ST_DWithin(geog, ST_SetSRID(ST_MakePoint(34.77, -0.09), 4326)::geography, 50000)")
total=$(( $(now) - start ))

result="PASS"
[ "$failures" -eq 0 ] || result="FAIL ($failures check(s) failed)"
cat <<REPORT
# Restore drill $(date -u +%Y-%m-%d)

- **Result:** $result
- **When:** $(date -u +"%Y-%m-%d %H:%M UTC")
- **Source:** $(echo "$SOURCE_DATABASE_URL" | sed -E 's#://[^@]*@#://***@#')
- **Backup:** custom-format \`pg_dump\`, $size; dump took ${dump_seconds}s
- **Restore:** into a scratch \`$image\` container, ${restore_seconds}s (\`pg_restore -j 4\`)
- **Total time:** ${total}s
- **Schema version:** source $src_version, restored $dst_version; PostGIS $postgis
- **Radius query on restored data** (50 km of Kisumu): $spatial report(s)

$(printf "%b" "$rows")
REPORT
[ "$failures" -eq 0 ]
