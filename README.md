# AuraRisk
AuraRisk is a climate-risk platform providing localized flood predictions, early warnings, and community-driven disaster reporting. By combining weather forecasts, satellite data, geospatial mapping, and citizen insights, it helps communities, governments, NGOs, and businesses proactively prepare for flood events.

## Backend setup and run

This project includes a Go backend that exposes weather-risk and community-report APIs. The backend was verified to start successfully with PostgreSQL running locally.

### Prerequisites
- Go 1.22+
- Docker and Docker Compose
- PostgreSQL (provided by the included Compose setup)

### Quick start

1. Start the local Postgres database from the repo root:

```bash
cd /workspaces/Aura-Risk
docker compose up -d postgres
```

2. Configure the backend environment variables. You can either copy the example file or export the values manually:

```bash
cd /workspaces/Aura-Risk
cp .env.example .env
```

Edit `.env` and replace the `change-me-*` placeholders with your own local passwords. Docker Compose reads the same file and refuses to start if `POSTGRES_PASSWORD`, `AWS_ACCESS_KEY_ID`, or `AWS_SECRET_ACCESS_KEY` are missing. The backend has no built-in database URL and exits at startup if `DATABASE_URL` is not set.

See [Secrets and production configuration](docs/ARCHITECTURE.md#secrets-and-production-configuration) for `APP_ENV=production`, mounted secret files, and AWS Secrets Manager.

3. Run the backend:

```bash
cd /workspaces/Aura-Risk/aurarisk-backend
go mod tidy
go run .
```

4. Confirm the server is running:

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok"}
```

### API endpoints
- GET `/health` — health check
- GET `/api/risk?lat=...&lon=...` — flood risk assessment
- GET `/api/reports?lat=...&lon=...` — nearby community reports
- POST `/api/reports` — create a new report
- Report moderation, duplicate detection, automatic verification, and retention — see [docs/REPORT_CURATION.md](docs/REPORT_CURATION.md)
- Device accounts, push consent, location subscriptions, and photo uploads — see [docs/MOBILE_PHASE3.md](docs/MOBILE_PHASE3.md)

### Monitoring and on-call

`docker compose --profile monitoring up -d` starts Prometheus, Alertmanager, Grafana (http://localhost:3000), and a Postgres exporter. Set `METRICS_ADDR=:9464` in `.env` so Prometheus can scrape the backend. Dashboards cover API latency, DB saturation, provider health, and weather data freshness. Alert rules, ownership, escalation, and runbooks are in [docs/ON_CALL.md](docs/ON_CALL.md).

### Verified status
The backend was verified to build and run successfully with the database available. The application logs show:

```text
✅ Database connected
✅ Migrations complete
🚀 AuraRisk backend running on port 8080
```

### Troubleshooting
- If PostgreSQL is not reachable, start it again with `docker compose up -d postgres`.
- If the app cannot connect to the database, check that the `DATABASE_URL` matches the local Postgres container. Postgres only applies `POSTGRES_PASSWORD` when it first creates its volume, so an existing `aurarisk_pgdata` volume keeps its old password; either keep using it in `.env` or run `docker compose down -v` to recreate the database.
- If a dependency issue occurs, run `go mod tidy` inside `aurarisk-backend`.

## Web and mobile clients

- `aurarisk-frontend/`: Vite + React web app (`npm install && npm run dev`). It calls the backend through the dev server's `/api` proxy, so start the backend first. Set `VITE_API_BASE_URL` for builds that talk to an API on another origin (and list the app in the backend's `CORS_ALLOWED_ORIGINS`), or `VITE_USE_MOCK_API=true` to work on the UI without a backend. See `aurarisk-frontend/.env.example`.
- `aurarisk-mobile/`: Expo app (`npm install && npx expo start`). Set `EXPO_PUBLIC_API_BASE_URL` to an address the device can reach (for example `http://10.0.2.2:8080` on the Android emulator).
- `packages/shared/`: API types, client, and validation used by both. See [its README](packages/shared/README.md).

The mobile app picks a location from the device's GPS when the user allows it. If permission is denied, blocked, or location services are off, the user can still search for a place or type coordinates.

## Contributing

- Every pull request runs CI (`.github/workflows/ci.yml`): Go formatting, vet, and tests (including Postgres integration tests), the mobile typecheck and tests (which cover `packages/shared`), the web build, and the monitoring config checks.
- Link each PR to its issue. `Closes #N` closes the issue on merge; use `Part of #N` when only some deliverables are done. The PR template has a checklist for this.
- Close issues through PRs rather than by hand, so every closed issue points at the code that implemented it.
