# AuraRisk Architecture and Production Guide

## What the app does

AuraRisk combines location search, map visualization, weather-based flood risk, and community reports. A user selects a location or uses device geolocation, then the UI loads a risk assessment and nearby reports for that point.

The current implementation is a prototype. The web frontend calls the Go backend, which provides the weather-risk and community-report APIs.

## Repository layout

- `aurarisk-frontend/`: React, TypeScript, Vite, Tailwind, and MapLibre web client.
- `aurarisk-backend/`: Go HTTP API using Gin and PostgreSQL.
- `aurarisk-backend/internal/services/`: Open-Meteo integration and risk scoring.
- `aurarisk-backend/internal/handlers/`: HTTP handlers for risk and reports.
- `aurarisk-backend/internal/database/`: PostgreSQL connection and startup migration.
- `migrations/`: SQL schema reference.
- `docker-compose.yml`: local PostgreSQL service only.

## Request flow

1. The user searches for a location through Nominatim, selects a map point, or grants browser geolocation access.
2. `App.tsx` stores the selected location and requests risk and reports in parallel.
3. The frontend calls `/api/risk?lat=<lat>&lon=<lon>` and `/api/reports?lat=<lat>&lon=<lon>&radius=10` when backend mode is enabled.
4. The backend gets Open-Meteo forecast data for the coordinates' ~2 km grid cell. It requests seven days of hourly precipitation, rain, and `soil_moisture_0_to_7cm`, plus current weather values. Results are cached per cell for 15 minutes and concurrent requests for a cell share one fetch (`internal/services/weather.go`).
5. The risk service combines current precipitation, the next six forecast hours, antecedent rainfall, recent soil moisture, weather severity, and a terrain placeholder factor into a score from 0 to 100.
6. The frontend renders the score, risk level, tips, reports, and map markers.
7. New reports are validated by the backend and inserted into PostgreSQL.

## Risk calculation

The current score is heuristic, not a calibrated flood model. It includes:

- Current precipitation multiplied by 10.
- The next six forecast precipitation values multiplied by 5.
- Preceding 48-hour rainfall, capped at 20 points.
- Preceding seven-day rainfall, capped at 15 points.
- Recent 0-7 cm soil moisture, capped at 20 points.
- Current weather-code severity.
- A temporary coordinate-based terrain factor.

The Open-Meteo history window is controlled by `historicalDays` in `aurarisk-backend/internal/services/openmeteo.go`. Keep that value synchronized with the scoring assumptions if changing it. The current default is seven days, giving 168 historical hourly values plus one forecast day.

For a production flood warning, replace or calibrate the heuristic with local rainfall gauges, stream or river levels, elevation and drainage data, soil type, land cover, basin boundaries, and historical flood labels. Validate thresholds separately for each region and publish the model version with every assessment.

## Running locally

### Backend and database

```bash
docker compose up -d postgres
cd aurarisk-backend
go run .
```

The backend reads `DATABASE_URL` and `PORT` from the environment (or a `.env` in `aurarisk-backend/` or the repo root). There is no fallback database URL: startup fails if `DATABASE_URL` is missing.

### Secrets and production configuration

`APP_ENV` is `development` (default) or `production`; any other value aborts startup. In production the backend also refuses to start when:

- `DATABASE_URL` contains a development password (`password`, `postgres`, or a `change-me*` placeholder).
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, or `EXPO_ACCESS_TOKEN` hold a development value.
- `EXPO_ACCESS_TOKEN` is empty while the notifier is enabled (`NOTIFIER_INTERVAL` not `0`).

In every environment `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` must be set together or not at all (leave both unset to use an IAM role). All problems are reported together.

Each secret variable (`DATABASE_URL`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `EXPO_ACCESS_TOKEN`) can be supplied in one of three ways:

| Form | Example | Notes |
|---|---|---|
| Plain value | `DATABASE_URL=postgres://...` | Local development |
| Mounted file | `DATABASE_URL_FILE=/run/secrets/database_url` | Docker/Kubernetes secrets; trailing newline is trimmed. Setting both `KEY` and `KEY_FILE` is an error. |
| AWS Secrets Manager | `DATABASE_URL=awssm://prod/aurarisk#database_url` | Secret name or ARN. `#field` selects a string field from a JSON secret; omit it to use the whole secret. |

Secrets Manager uses the standard AWS credential chain and `AWS_REGION` (or the region in an ARN), and needs `secretsmanager:GetSecretValue` on the referenced secrets. File references are resolved before Secrets Manager, so AWS credentials can come from files. Each secret is fetched once at startup, so rotating a value requires a restart.

### Frontend

```bash
cd aurarisk-frontend
npm install
npm run dev
```

The app calls the backend. In development, Vite proxies `/api` to `http://localhost:8080` (override with `API_PROXY_TARGET`). Settings are in `aurarisk-frontend/.env.example`:

- `VITE_API_BASE_URL`: backend origin, compiled in at build time. Leave it empty to serve the app and API from one origin behind a reverse proxy. If the API is on another origin, add the app's origin to the backend's `CORS_ALLOWED_ORIGINS`.
- `VITE_USE_MOCK_API=true`: mock data for UI work without a backend. Development only; production builds never include the mock code.

Build for production with `npm run build`.

## Current API

- `GET /health`: liveness-style health response.
- `GET /api/risk?lat=<lat>&lon=<lon>`: weather-based risk assessment.
- `GET /api/reports?lat=<lat>&lon=<lon>&radius=<km>`: up to 50 nearby reports.
- `POST /api/reports`: creates a community report.

The report endpoint expects `location`, `category`, and `note`. Valid categories are `flooding`, `road_blocked`, `water_rising`, and `drainage_issue`.

## Production readiness assessment

### Current status

The app is suitable for a development demo or pilot, but it should not yet be used as a safety-critical public warning system. It has a useful end-to-end product shape, but the prediction model, operational controls, security, observability, and deployment process need work.

### Launch blockers

1. ~~**Use real frontend mode.**~~ Done: the web app calls the backend, the API origin is a build-time setting (`VITE_API_BASE_URL`), and mock data is development-only and excluded from production builds.
2. ~~**Remove default secrets and credentials.**~~ Done: Compose and the backend no longer ship credentials, production startup fails on missing or development secrets, and secrets can come from mounted files or AWS Secrets Manager (see [Secrets and production configuration](#secrets-and-production-configuration)).
3. **Secure report creation.** Add authentication or abuse controls, request size limits, strict latitude/longitude validation, note length limits, spam protection, moderation, and audit logging.
4. **Add database migrations.** Run versioned migrations as a deployment step instead of embedding schema creation in application startup. Add constraints and indexes appropriate for geographic queries.
5. **Harden the API.** Add structured error responses, request IDs, CORS allowlists, rate limiting, timeouts, graceful shutdown, and health checks that distinguish process health from database and provider health.
6. ~~**Make weather access resilient.**~~ Done: grid-cell caching with request coalescing, per-attempt timeouts and jittered retries, a circuit breaker, an outgoing call budget, and a stale-data policy (cached data up to `WEATHER_STALE_MAX` old is served with `stale: true`; otherwise `/api/risk` returns 503 with `Retry-After`). The notifier never alerts from stale data. Paid Open-Meteo plans are supported via `OPEN_METEO_API_KEY`. Originally: add caching by coordinate grid and time window, provider timeouts and retries with backoff, circuit breaking, and a stale-data policy. Open-Meteo and Nominatim usage must follow their service policies and rate limits.
7. **Calibrate the risk model.** The terrain factor is a placeholder and the score has no regional validation. Do not present it as an official warning until it is evaluated against observed events and reviewed by domain experts.
8. **Add tests and CI.** Cover score edge cases, malformed API inputs, database failures, Open-Meteo decoding, handler responses, and frontend builds. Run formatting, static analysis, tests, and dependency checks on every pull request.
9. **Deploy the frontend and backend separately.** Configure HTTPS, a real reverse proxy or API gateway, immutable builds, environment-specific settings, backups, restore drills, and rollback procedures.

### High-value next improvements

- ~~Add Prometheus-compatible metrics.~~ Done (see [ON_CALL.md](ON_CALL.md)). Still to do: centralized logs with correlation IDs.
- Expose data freshness in the UI. Provider response age is now tracked as `aurarisk_weather_observation_age_seconds`.
- Return confidence and model version with risk assessments. (Weather source timestamps are now returned as `source_timestamps`.)
- Use PostGIS or a geospatial index for accurate radius searches instead of a latitude/longitude bounding box.
- ~~Add report verification, duplicate detection, moderation status, and retention policies.~~ Done: see [REPORT_CURATION.md](REPORT_CURATION.md).
- Add accessible loading, empty, error, and offline states in the frontend.
- ~~Add monitoring alerts for API latency, provider failures, database saturation, and stale weather data.~~ Done: dashboards, alert rules, routing, and on-call ownership are in `observability/` and [ON_CALL.md](ON_CALL.md).

## Mobile app direction

The backend can serve a mobile client without a separate mobile-specific API. Keep the risk and report contracts stable, then build a React Native or Expo client that shares TypeScript types, API clients, validation schemas, and design tokens with the web app.

Recommended mobile capabilities:

- Permission-aware location selection with a manual fallback.
- Cached last-known risk and reports for poor connectivity.
- Push notifications for subscribed locations, with explicit consent and quiet hours.
- Background refresh only where platform policy and battery budgets allow it.
- Offline report composition and a retry queue.
- Photo attachments with server-side resizing, malware scanning, and object storage rather than database blobs.
- Secure token storage and account/device management.
- Accessible map controls and a list-based alternative for screen readers.

A practical sequence is: stabilize the HTTP API and authentication, extract shared TypeScript contracts, ship a small Expo client using the existing endpoints, then add push notifications and offline synchronization after the online workflow is reliable.

## Recommended release gates

Before a public launch, require:

- A validated model report for each launch region.
- No default production credentials.
- HTTPS and authenticated or abuse-protected report submission.
- Automated tests and CI passing.
- Database backup and restore verification.
- Weather provider caching, timeout, fallback, and freshness indicators.
- Monitoring dashboards and on-call ownership.
- A documented incident and rollback procedure.
- Clear UI language stating that AuraRisk is decision support unless an authorized agency designates it as an official warning service.
