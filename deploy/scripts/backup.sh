#!/usr/bin/env sh
# Nightly logical backup of the AuraRisk database, for hosts without managed
# backups (managed Postgres snapshots are still the first line). Run from cron:
#   15 2 * * * DATABASE_URL=... BACKUP_S3_URI=s3://bucket/aurarisk deploy/scripts/backup.sh
# Keeps BACKUP_KEEP_DAYS (14) days locally; uploads to S3 when BACKUP_S3_URI is set.
set -eu

: "${DATABASE_URL:?set DATABASE_URL}"
dir="${BACKUP_DIR:-$(dirname "$0")/../backups}"
keep="${BACKUP_KEEP_DAYS:-14}"
mkdir -p "$dir"

file="$dir/aurarisk-$(date -u +%Y%m%dT%H%M%SZ).dump"
# Custom format: compressed, and pg_restore can restore it in parallel.
pg_dump --format=custom --no-owner --no-privileges --file="$file" "$DATABASE_URL"
pg_restore --list "$file" > /dev/null # fails on a truncated or corrupt dump
echo "backup written: $file ($(du -h "$file" | cut -f1))"

if [ -n "${BACKUP_S3_URI:-}" ]; then
  aws s3 cp --only-show-errors "$file" "$BACKUP_S3_URI/$(basename "$file")"
  echo "uploaded to $BACKUP_S3_URI"
fi

find "$dir" -name 'aurarisk-*.dump' -mtime +"$keep" -delete
