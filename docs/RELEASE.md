# Releases, rollback and incidents

How a change reaches users, how to undo it, and what to do when something goes
wrong in production (#27). Alerts and their runbooks are in
[ON_CALL.md](ON_CALL.md); hosting options are in [DEPLOYMENT.md](DEPLOYMENT.md).

## Builds are immutable

- CI builds the backend image on every PR. On every push to `main`, it also
  publishes the image to GitHub Container Registry as
  `ghcr.io/iochieng1/aurarisk-backend:sha-<full commit SHA>`, plus a moving
  `main` tag.
- A `sha-` tag is never rebuilt or reused, so a tag identifies exactly one
  build. Deploy by `sha-` tag, never by `main`.
- The commit is compiled into the binary: `GET /health` returns
  `{"status": "ok", "version": "<sha>"}`. Use it to confirm what is serving
  after a deploy or rollback.
- The web app is a static build. Production serves the `dist/` built from the
  same commit as the API image.

## Environments

| Environment | Where | Settings |
|---|---|---|
| Local | `docker-compose.yml` + `go run .` | `.env.example` |
| Demo | Render + Neon, deployed from `main` automatically | `render.yaml` |
| Staging | `deploy/docker-compose.prod.yml` on its own server and database | `deploy/env/staging.env.example` |
| Production | `deploy/docker-compose.prod.yml` behind Caddy (HTTPS), or the managed equivalent in DEPLOYMENT.md | `deploy/env/production.env.example` |

`APP_ENV=production` makes the API refuse to start with missing or development
secrets (`config.Validate`). Real env files are git-ignored. On AWS, use
Secrets Manager references (`awssm://`) instead of plain values.

## HTTPS and the proxy

On a single server, `deploy/docker-compose.prod.yml` runs:
- **Caddy**, the only container with public ports. It gets and renews Let's
  Encrypt certificates for `API_HOST` and `WEB_HOST`, redirects HTTP to HTTPS,
  and adds HSTS and security headers.
- **The API**, which listens only on the compose network.
  - Caddy health-checks `/health/ready` and stops routing to an instance that
    is draining.
  - `TRUSTED_PROXIES` covers the compose network, so rate limits see real
    client IPs.

On AWS, an ALB with an ACM certificate takes Caddy's place.

## Releasing

1. Merge to `main` with CI green. Branch protection requires every CI job.
2. Wait for the **Publish image** job, then note the tag `sha-<commit>`.
3. **Staging:** deploy that tag and check it:

   ```sh
   cd deploy
   IMAGE_TAG=sha-<commit> ENV_FILE=env/staging.env \
     docker compose -p aurarisk-staging -f docker-compose.prod.yml --env-file env/staging.env up -d
   curl -s https://api.staging.aurarisk.example/health   # version matches
   ```

   The `migrate` service applies pending migrations before the API starts.
   If a migration fails, the API doesn't start and the old one keeps
   serving.
4. **Production:** the same command with `env/production.env` and the same tag.
   Record the tag and time in `#aurarisk-alerts`.
5. Watch the Grafana *Service overview* for 15 minutes: error rate, latency,
   and weather freshness.

The demo deploys itself: Render builds `main` on every merge and runs
migrations at startup (`AUTO_MIGRATE=true`).

### Migrations must be backward compatible

Rollback swaps the image, not the schema, so every release must work with the
schema **before and after** its migrations:
- Add columns as nullable or with a default. Add tables freely.
- Remove or rename in two releases: first stop using the column, then drop it
  in a later release.
- Never edit a migration that has shipped. Add a new one.

Each migration file says how to reverse it by hand, for the rare case it must
be undone. Prefer a new forward migration.

## Rolling back

Roll back first and investigate later when a release causes errors, wrong
risk results, or missing alerts.

| Where | How |
|---|---|
| Single server | Deploy the previous tag: `IMAGE_TAG=sha-<previous> docker compose ... up -d`. Find it in `#aurarisk-alerts` or with `docker image ls ghcr.io/iochieng1/aurarisk-backend` |
| Render (demo) | Dashboard → `aurarisk-api` → *Events* → the previous deploy → **Rollback**. Same for `aurarisk-web` |
| Web app only | Redeploy the previous `dist/` build (or Render rollback) |

Then:
1. Check `GET /health` shows the previous version, and `/health/ready` is 200.
2. Check the Grafana error rate is back to normal.
3. Revert the commit on `main`, so the next deploy doesn't bring the problem
   back. Open an issue.

**Restore the database only for data loss or corruption**, never to undo a
release. A restore loses every report, subscription and moderation decision
since the backup.

## Incidents

[ON_CALL.md](ON_CALL.md) covers severities, paging and per-alert runbooks. In
short:

1. **Acknowledge and announce** in `#aurarisk-alerts`: what's affected, who's
   on it.
2. **Mitigate first:** roll back a recent release; restart; disable a failing
   feature by its setting:
   - `NOTIFIER_INTERVAL=0`
   - unset `CHANNEL_CALLBACK_SECRET`
   - unset `COUNTY_WEBHOOKS`
3. **Communicate** every 30 minutes while users are affected. If risk data is
   wrong or missing during heavy rain, say so publicly (web banner or social
   channels) and point people to official Kenya Meteorological Department
   warnings.
4. **Resolve:** confirm the alert clears and the dashboards are normal.
5. **Postmortem** within 5 working days for any user-facing outage over 15
   minutes, any missed flood alert, and any restore from backup. Cover the
   timeline, impact, cause, and actions with owners.

## Backups and restore drills

- **Managed Postgres backups are the first line.**
  - Neon keeps point-in-time history for its retention window.
  - RDS and DigitalOcean take daily snapshots with point-in-time recovery.
  - Turn these on in every environment.
- **Logical backups** with `deploy/scripts/backup.sh` (nightly from cron;
  uploads to S3 when `BACKUP_S3_URI` is set). They protect against losing the
  provider account and allow restoring to another host.
- **Drill:** `deploy/scripts/restore-drill.sh` restores a backup into a
  scratch PostGIS container. It checks the schema version, PostGIS, row
  counts for every table against the source, and a radius query. It prints a
  Markdown record.

  ```sh
  SOURCE_DATABASE_URL='postgres://readonly@prod-host/aurarisk?sslmode=require' \
    deploy/scripts/restore-drill.sh > docs/drills/$(date -u +%F)-restore-drill.md
  ```

  Commit the record. Run the drill **before launch**, then **quarterly**, and
  after any change to backups or the database version. Use a read-only role
  for `SOURCE_DATABASE_URL`. If the drill fails, open an issue and treat it
  as a release blocker.

| Date | Source | Result | Time | Record |
|---|---|---|---|---|
| 2026-10-28 | Synthetic rehearsal (100k reports) | Pass | 43 s total, 9 s restore | [record](drills/2026-10-28-restore-drill.md) |
| — | **Production backup** | Pending: run once production hosting exists | | |
