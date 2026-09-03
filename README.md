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

The example file contains:

```env
PORT=8080
DATABASE_URL=postgres://postgres:password@localhost:5432/aurarisk?sslmode=disable
```

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

### Verified status
The backend was verified to build and run successfully with the database available. The application logs show:

```text
✅ Database connected
✅ Migrations complete
🚀 AuraRisk backend running on port 8080
```

### Troubleshooting
- If PostgreSQL is not reachable, start it again with `docker compose up -d postgres`.
- If the app cannot connect to the database, check that the `DATABASE_URL` matches the local Postgres container.
- If a dependency issue occurs, run `go mod tidy` inside `aurarisk-backend`.

