# Production deployment options

Where and how to run AuraRisk in production
([#27](https://github.com/iochieng1/Aura-Risk/issues/27)). This is a
recommendation, not a finished setup: the repo has no Dockerfiles or
infrastructure code yet.

## What has to run

| Component | Notes |
|---|---|
| Backend (Go) | API on `PORT`; `/metrics` on a private `METRICS_ADDR`. Run `aurarisk-backend migrate` as a deploy step before new instances start. |
| PostgreSQL 16 | Add PostGIS for [#18](https://github.com/iochieng1/Aura-Risk/issues/18). Needs automated backups and a tested restore. |
| Object storage (S3 API) | Photo uploads (`S3_*` settings). |
| ClamAV (clamd) | Scans photos before they are published. Needs roughly 2–3 GB of memory for its signature database. |
| Web app | Static files from `npm run build`. After [#45](https://github.com/iochieng1/Aura-Risk/pull/45), `.mjs` files must be served as `text/javascript`. |
| Monitoring | Metrics, logs, alerts. Self-hosted (`observability/`) or a managed service. |
| Secrets | `DATABASE_URL`, AWS keys, `MODERATOR_TOKENS`, `EXPO_ACCESS_TOKEN`, ... Read from env, `<KEY>_FILE` mounts, or AWS Secrets Manager (`awssm://`). |

## Region

Pick the cloud region closest to users. For a Philippines launch (the web map
defaults to Manila), that is **Singapore** on the major clouds.

## Option A: AWS, Singapore (`ap-southeast-1`), recommended for launch

The backend already uses AWS Secrets Manager and S3, so this needs no code
changes.

| Component | Service |
|---|---|
| Backend | ECS on Fargate behind an Application Load Balancer, with an ACM certificate for HTTPS. Run the migration as a one-off task before each deploy. |
| ClamAV | A separate ECS service on the private network; set `CLAMD_ADDR` to it. |
| Database | RDS for PostgreSQL (supports PostGIS), Multi-AZ when the budget allows. Automated backups and point-in-time restore. |
| Photos | S3, private bucket; access through presigned URLs as now. |
| Secrets | Secrets Manager with `awssm://` references; the task role grants read access. |
| Web app | S3 + CloudFront. |
| Monitoring | Grafana Cloud (free tier to start): Alloy sends metrics and logs, and the existing dashboards and alert rules can be imported. Or self-host `observability/` on a small instance. |

- **Pros:** managed backups and failover, autoscaling, and it fits the
  existing code.
- **Cons:** the most setup and the highest base cost: roughly
  $60–120/month to start. Check current pricing.

## Option B: DigitalOcean, Singapore (`SGP1`), cheaper for an MVP

| Component | Service |
|---|---|
| Backend + ClamAV | App Platform, or a Droplet running Docker Compose |
| Database | Managed PostgreSQL (PostGIS available), daily backups |
| Photos | Spaces (S3-compatible): set `S3_ENDPOINT` and `S3_FORCE_PATH_STYLE=true`, already supported |
| Secrets | App Platform encrypted env vars, or `<KEY>_FILE` mounts on a Droplet |
| Web app | App Platform static site, or Cloudflare Pages |
| Monitoring | Grafana Cloud free tier |

- **Pros:** simpler and cheaper, roughly $40–60/month to start. Check current
  pricing.
- **Cons:** no Secrets Manager integration, fewer managed options for
  failover.

## Other options

- **Fly.io / Render / Railway:** quick to start, but check that they have a
  nearby region and managed Postgres with PostGIS before choosing.
- **Web app only:** Cloudflare Pages, Vercel or Netlify all work for the
  static build with any backend option. Make sure `.mjs` is served as
  JavaScript.

## Work needed before any option

1. **Dockerfiles**: a multi-stage build for the backend (one static binary
   that also serves `migrate`), and a build for the web app.
2. **Image builds in CI**: images tagged by commit SHA, so a release is
   immutable and rollback means deploying the previous tag.
3. **Production settings**:
   - `APP_ENV=production` (logs default to JSON) and `TRUSTED_PROXIES` set to
     the load balancer.
   - `CORS_ALLOWED_ORIGINS` set to the web app's origin.
   - `SHUTDOWN_DRAIN_DELAY` set above the load balancer's health-check
     interval.
   - Health checks on `/health/ready`.
4. **Infrastructure as code** (Terraform or similar), so environments can be
   rebuilt.
5. **Release gates from #27**:
   - **Runbooks:** document the incident and rollback procedures, building on
     [ON_CALL.md](ON_CALL.md).
   - **Restore drill:** restore a database backup into a scratch instance,
     check it, and record the date and time taken. Repeat this regularly.

## Recommendation

Use **AWS Singapore** for the public launch: it matches the code's existing
integrations and covers backups, HTTPS and secrets with managed services. If
budget is the main constraint for an early pilot, **DigitalOcean Singapore**
works with only configuration changes. Use Grafana Cloud for monitoring on
either. Start with the Dockerfiles, which every option needs.
