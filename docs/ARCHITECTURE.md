# AuraRisk Architecture and Production Guide

## What the app does

AuraRisk combines location search, map visualization, weather-based flood risk, and community reports. A user selects a location or uses device geolocation, then the UI loads a risk assessment and nearby reports for that point.

The current implementation is a prototype. The frontend defaults to local mock data, while the Go backend provides the real weather and report APIs when enabled.

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
4. The backend fetches Open-Meteo forecast data for the coordinates. It requests seven days of hourly precipitation, rain, and `soil_moisture_0_to_7cm`, plus current weather values.
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

The backend reads `DATABASE_URL` and `PORT` from the environment. The local fallback database URL is intended only for development.

### Frontend

```bash
cd aurarisk-frontend
npm install
npm run dev
```

The Vite development server proxies `/api` to `http://localhost:8080`. To use the real backend, change `USE_BACKEND` to `true` in `src/api/risk.ts`. A production build can be generated with `npm run build`.

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

1. **Use real frontend mode.** Replace the mock default with a build-time API base URL and environment-specific configuration. Avoid compiling mock behavior into a production build.
2. **Remove default secrets and credentials.** The Compose file contains a development database password, and the backend has a fallback DSN. Use a secret manager and fail startup when production configuration is missing.
3. **Secure report creation.** Add authentication or abuse controls, request size limits, strict latitude/longitude validation, note length limits, spam protection, moderation, and audit logging.
4. **Add database migrations.** Run versioned migrations as a deployment step instead of embedding schema creation in application startup. Add constraints and indexes appropriate for geographic queries.
5. **Harden the API.** Add structured error responses, request IDs, CORS allowlists, rate limiting, timeouts, graceful shutdown, and health checks that distinguish process health from database and provider health.
6. **Make weather access resilient.** Add caching by coordinate grid and time window, provider timeouts and retries with backoff, circuit breaking, and a stale-data policy. Open-Meteo and Nominatim usage must follow their service policies and rate limits.
7. **Calibrate the risk model.** The terrain factor is a placeholder and the score has no regional validation. Do not present it as an official warning until it is evaluated against observed events and reviewed by domain experts.
8. **Add tests and CI.** Cover score edge cases, malformed API inputs, database failures, Open-Meteo decoding, handler responses, and frontend builds. Run formatting, static analysis, tests, and dependency checks on every pull request.
9. **Deploy the frontend and backend separately.** Configure HTTPS, a real reverse proxy or API gateway, immutable builds, environment-specific settings, backups, restore drills, and rollback procedures.

### High-value next improvements

- Add Prometheus-compatible metrics and centralized logs with correlation IDs.
- Track provider response age and expose data freshness in the UI.
- Return confidence, source timestamps, and model version with risk assessments.
- Use PostGIS or a geospatial index for accurate radius searches instead of a latitude/longitude bounding box.
- Add report verification, duplicate detection, moderation status, and retention policies.
- Add accessible loading, empty, error, and offline states in the frontend.
- Add monitoring alerts for API latency, provider failures, database saturation, and stale weather data.

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
