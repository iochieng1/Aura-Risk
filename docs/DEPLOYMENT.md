# Production deployment options

Where and how to run AuraRisk
([#27](https://github.com/iochieng1/Aura-Risk/issues/27)).

- **[Free portfolio deployment](#free-portfolio-deployment-render--neon):**
  runs today at no cost, using `render.yaml` and `aurarisk-backend/Dockerfile`.
- **[Production options](#what-has-to-run):** recommendations for a real
  launch. These still need infrastructure code.

## Free portfolio deployment (Render + Neon)

A public demo for $0: the web app and API on Render's free plan in Singapore,
and Postgres on Neon's free plan. No card is needed for either (check their
current free-tier terms). The free Open-Meteo API is fine here, because a
portfolio demo is non-commercial.

**Turned off in the demo:**
- **Photos:** ClamAV needs more memory than free plans give. With `S3_BUCKET`
  unset, uploads are disabled safely.
- **Push alerts:** `NOTIFIER_INTERVAL=0`, because they need an Expo access token.

### 1. Database (Neon)

1. Sign up at neon.tech and create a project in **AWS Asia Pacific (Singapore)**.
2. Copy the connection string. It looks like
   `postgres://user:password@ep-xxx.ap-southeast-1.aws.neon.tech/neondb?sslmode=require`.

### 2. API and web app (Render)

1. In Render, choose **New → Blueprint**, select this repository and branch
   `main`. Render reads `render.yaml` and proposes two services:
   `aurarisk-api` (Docker) and `aurarisk-web` (static site).
2. Fill in the values it asks for:
   - `DATABASE_URL`: the Neon connection string.
   - `CORS_ALLOWED_ORIGINS`: `https://aurarisk-web.onrender.com`
   - `VITE_API_BASE_URL`: `https://aurarisk-api.onrender.com`

   If Render gives a service a different URL (when the name is taken), use
   the URLs shown on each service's page instead.
3. Apply. The API applies the database migrations on its first start
   (`AUTO_MIGRATE=true`).
4. If you changed a URL after the first deploy, update the variable and
   redeploy that service. `VITE_API_BASE_URL` is read when the web app is
   built, so the web app needs a new build after it changes.

### 3. Check it

- `https://<api>/health/ready` returns `{"status":"ok", ...}`.
- Open the web app, use **My Location** or search a town, and confirm the
  risk card and reports load.
- In the API's Render logs, the JSON access log's `client_ip` should be your
  own IP address. If it shows `::1`, `127.0.0.1` or a `10.x` address,
  Render's proxy isn't covered by `TRUSTED_PROXIES`, so every visitor shares
  one rate limit. Set it to `127.0.0.1,::1,10.0.0.0/8` (the value in
  `render.yaml`) and redeploy.

### Free-tier limits

- **Weather on a shared IP:** Render's free services share outgoing IP
  addresses, which often exceed Open-Meteo's free daily limit ("Daily API
  request limit exceeded" in the logs). Then `GET /api/risk` answers 503 and
  the apps fall back to fetching the weather on the visitor's device: each
  visitor uses their own allowance, and the server still validates and scores
  the data (`POST /api/risk`). The risk card's confidence reasons say when
  this happened.

- **Sleeping:** the API sleeps after about 15 minutes idle, so the first
  request then takes up to a minute. The web app shows its loading state
  meanwhile. Say so next to the demo link.
- **Storage:** Neon's free storage is small; it's plenty for a demo.
- **Auto-deploy:** each push to `main` redeploys only the services whose files
  changed (`buildFilter` in `render.yaml`).

### Mobile app

Point `EXPO_PUBLIC_API_BASE_URL` at the API URL, then share it either way:
- run `npx expo start` and show it in Expo Go, or
- build an Android APK with EAS's free plan.

A short screen recording is often the easiest way to show it in a portfolio.

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

1. **Images**: the backend Dockerfile exists (`aurarisk-backend/Dockerfile`;
   the same binary runs `migrate`). The web app is a static build and needs
   none.
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
either. Until then, the free Render + Neon demo above shows the app working.
