# On-call and incident response

How AuraRisk is monitored, who responds when something breaks, and what to do
for each alert. Alert rules link to the runbook sections below, so keep the
headings in sync with `observability/prometheus/alerts.yml`.

## Ownership

| Area | Component label | Owner | Covers |
|---|---|---|---|
| API | `api` | Backend team | HTTP availability, error rate, latency |
| Database | `database` | Backend team | Connection pool, Postgres availability and connection limits |
| Weather data | `weather` | Backend team | Open-Meteo availability and data freshness |
| Push alerts | `push` | Backend team | Risk notifier and Expo push delivery |
| Photos | `photos` | Backend team | S3 object storage, ClamAV scanning, photo processor |
| Report curation | `reports` | Backend team | Duplicate detection, automatic verification, retention cleanup |
| Report moderation decisions | — | Moderators (`MODERATOR_TOKENS`) | Reviewing the pending queue; not paged |
| Mobile and web clients | — | Mobile/web team | Client crashes and releases (not paged from these alerts) |

**Service owner:** @iochieng1. The service owner keeps the rotation staffed,
owns the alert rules and this document, and reviews every page in the weekly
handoff.

Every alert carries `team` and `component` labels. The `team` label is who
fixes the underlying problem; the on-call engineer is who responds first.

## Rotation

- **Primary on-call** is paged for every `severity="page"` alert, 24/7.
  The rotation is weekly and hands off on Monday at 09:00 team local time.
- **Secondary on-call** is the previous week's primary. They are paged if the
  primary does not acknowledge within 15 minutes.
- **Service owner** is the final escalation if neither acknowledges within
  30 minutes.
- Swaps are fine; update the schedule in PagerDuty before the shift starts.

The PagerDuty service's escalation policy encodes the three levels above.
Alertmanager sends pages to it using the integration key in
`observability/alertmanager/secrets/pagerduty_routing_key`.

## Severities

| Severity | Meaning | Where it goes | Response |
|---|---|---|---|
| `page` | Users are not getting risk data or flood alerts, or will not soon | PagerDuty and `#aurarisk-alerts` | Acknowledge within 15 min, start mitigating immediately |
| `ticket` | Degraded or trending towards a page | `#aurarisk-alerts` only | Triage the next working day; open an issue if it needs work |

The worst failure for AuraRisk is a silent one: a flood alert that is never
sent. That is why the weather provider, push provider, and notifier alerts page
even at low traffic.

## When you are paged

1. **Acknowledge** in PagerDuty and say in `#aurarisk-alerts` that you're on it.
2. **Open the dashboard:** Grafana → AuraRisk → *Service overview*. The top row
   shows whether the API, database, weather data, and notifier are healthy.
3. **Follow the runbook** for the alert (linked from the notification).
4. **Mitigate first, then fix.** Restarting, scaling, or disabling a feature is
   fine if it restores service.
5. **Post updates** every 30 minutes while users are affected.
6. **Hand off** explicitly if your shift ends mid-incident.
7. **Write a postmortem** within 5 working days for any page that affected
   users for more than 15 minutes, or any missed flood alert.

## Running the monitoring stack

Locally:

```bash
# In .env: METRICS_ADDR=:9464 so Prometheus (in Docker) can scrape the backend
docker compose --profile monitoring up -d
cd aurarisk-backend && go run .
```

| UI | URL |
|---|---|
| Grafana (dashboard) | http://localhost:3000 |
| Prometheus (queries, alert states) | http://localhost:9090/alerts |
| Alertmanager (silences, routing) | http://localhost:9093 |

To deliver notifications locally, create
`observability/alertmanager/secrets/pagerduty_routing_key` and
`observability/alertmanager/secrets/slack_webhook_url` (git-ignored). Without
them, alerts still show in the Alertmanager UI and delivery failures are logged.

Check rule changes before merging:

```bash
docker run --rm -v "$PWD/observability/prometheus:/p:ro" --entrypoint promtool \
  prom/prometheus:v3.5.0 test rules /p/alerts_test.yml
```

In production:

- Bind `METRICS_ADDR` to a private interface (for example `10.0.0.5:9464`) and
  scrape every backend instance. Never expose it on the public load balancer.
- Use the same `alerts.yml` and `alertmanager.yml`; mount the two secret files
  from your secret manager.
- Run Grafana with real authentication. The local Compose setup's anonymous
  access is for development only.
- Keep `instances × DB_MAX_OPEN_CONNS` below Postgres `max_connections`.

## Metrics reference

| Metric | What it measures |
|---|---|
| `aurarisk_http_request_duration_seconds{method,route}` | API latency by route template |
| `aurarisk_http_requests_total{method,route,status}` | API requests by status |
| `go_sql_in_use_connections`, `go_sql_max_open_connections` | DB pool use per instance |
| `go_sql_wait_duration_seconds_total` | Time requests spent waiting for a pooled connection |
| `pg_stat_database_numbackends`, `pg_settings_max_connections` | Postgres server connections (postgres-exporter) |
| `aurarisk_provider_requests_total{provider,operation,outcome}` | Calls to `open_meteo`, `expo_push`, `s3`, `clamav` |
| `aurarisk_provider_request_duration_seconds{provider,operation}` | Provider latency |
| `aurarisk_provider_last_success_timestamp_seconds{provider}` | Last successful call per provider |
| `aurarisk_weather_observation_age_seconds` | Age of Open-Meteo's current conditions when fetched |
| `aurarisk_push_tickets_total{status}` | Per-message Expo results: `ok`, `device_gone`, `error` |
| `aurarisk_worker_last_success_timestamp_seconds{worker}` | Last successful notifier / photo processor / report verifier / retention run |
| `aurarisk_moderation_queue_reports` | Pending reports awaiting verification or review |
| `aurarisk_retention_deleted_total{kind,reason}` | Rows removed by the retention policy |

## Runbooks

### AuraRiskBackendDown

Prometheus cannot scrape a backend instance. The API and that instance's
background workers are probably down.

1. Check the process or container: is it running, crash-looping, or OOM-killed?
2. Read its logs from the last start. Startup aborts on bad config
   (`Startup aborted: ...`) or an unreachable database (`Failed to ping database`).
3. If Postgres is down too, follow [PostgresDown](#postgresdown) first.
4. Restart the instance. If it fails again, roll back the last deploy.

If the API answers on `/health` but this alert is firing, the metrics listener
is the problem: check `METRICS_ADDR` and network rules between Prometheus and
the instance.

### AuraRiskHighErrorRate

More than 5% of API requests are returning 5xx.

1. On the dashboard, find which route is failing (*Requests by status class*,
   then Explore: `sum by (route) (rate(aurarisk_http_requests_total{status=~"5.."}[5m]))`).
2. `/api/risk` only: almost always Open-Meteo. Check
   [AuraRiskWeatherProviderFailing](#aurariskweatherproviderfailing).
3. All routes: check the database panels and logs for SQL errors.
4. Started after a deploy: roll back.

### AuraRiskApiLatency

Covers `AuraRiskRiskEndpointSlow` (`/api/risk` p95 above 4s) and
`AuraRiskApiSlow` (any other route's p95 above 1s).

1. `/api/risk`: compare with *p95 latency by provider*. If Open-Meteo is slow,
   the API is slow; there is nothing to fix on our side except waiting or
   reducing call volume.
2. Other routes: check *Requests waiting for a connection*. Waiting means pool
   exhaustion; see [AuraRiskDbPoolSaturated](#aurariskdbpoolsaturated).
3. Otherwise look for slow queries in Postgres:
   `SELECT pid, now() - query_start AS age, state, query FROM pg_stat_activity WHERE state <> 'idle' ORDER BY age DESC;`

### AuraRiskDbPoolSaturated

Covers `AuraRiskDbPoolSaturated` (ticket, pool above 80% for 10 min) and
`AuraRiskDbPoolWaiting` (page, requests blocked on the pool).

1. Is it one instance or all? One instance points to a stuck worker or
   long-running request on it; restart that instance.
2. Look for long-running queries or idle-in-transaction sessions with the
   `pg_stat_activity` query above; terminate obvious offenders with
   `SELECT pg_terminate_backend(<pid>);`.
3. If traffic is legitimately higher, raise `DB_MAX_OPEN_CONNS` or add
   instances, but stay within Postgres `max_connections`
   (see [PostgresConnectionsNearLimit](#postgresconnectionsnearlimit)).

### PostgresDown

postgres-exporter cannot connect to Postgres. Every route that touches the
database, the notifier, and the photo processor are failing.

1. Check the database host or managed instance status and its logs.
2. Check disk space; a full disk stops Postgres.
3. Locally: `docker compose ps postgres` and `docker compose logs postgres`.
4. Once it is back, confirm the backend reconnects (errors stop) and the
   notifier completes a run.

### PostgresConnectionsNearLimit

Postgres is above 80% of `max_connections`. When it is full, new connections
(including new backend instances and deploys) fail.

1. Find who holds connections:
   `SELECT usename, application_name, client_addr, state, count(*) FROM pg_stat_activity GROUP BY 1,2,3,4 ORDER BY 5 DESC;`
2. Terminate leaked or idle-in-transaction sessions.
3. Lower `DB_MAX_OPEN_CONNS` per instance or add a pooler (PgBouncer) before
   scaling out further.

### AuraRiskWeatherProviderFailing

More than 25% of Open-Meteo calls are failing. `/api/risk` returns 500, and
the notifier skips subscriptions it cannot assess, so **flood alerts are not
being evaluated**.

1. Check https://open-meteo.com status and whether the failures are timeouts
   or HTTP errors (backend logs: `failed to fetch weather data`,
   `weather API returned status ...`).
2. HTTP 429: we are rate-limited. Reduce traffic or move to a commercial
   Open-Meteo plan / API key.
3. Timeouts or DNS errors from only our hosts: check egress networking.
4. Post in `#aurarisk-alerts` that risk data is unavailable. If the outage is
   long and there is an active flood event, tell the service owner so they can
   alert partners through other channels.

### AuraRiskWeatherDataStale

Covers `AuraRiskWeatherDataStale` (page: no successful fetch in 30 min while
weather is being requested) and `AuraRiskWeatherObservationsOld` (ticket:
fetches succeed but Open-Meteo's current conditions are over 2 hours old).

1. Stale fetches: treat as
   [AuraRiskWeatherProviderFailing](#aurariskweatherproviderfailing). The
   provider alert may be inhibited if it is already firing.
2. Old observations: Open-Meteo is serving outdated model data. Confirm with a
   manual request, for example
   `curl 'https://api.open-meteo.com/v1/forecast?latitude=-1.29&longitude=36.82&current=precipitation'`
   and compare `current.time` (local time, `utc_offset_seconds`) with now.
   Report it to Open-Meteo; nothing to restart on our side.

### AuraRiskPushProviderFailing

Covers `AuraRiskPushProviderFailing` (page: Expo push requests failing) and
`AuraRiskPushTicketErrors` (ticket: Expo accepts requests but rejects messages).

1. Check https://status.expo.dev.
2. HTTP 401/403 or ticket errors such as `InvalidCredentials`: the
   `EXPO_ACCESS_TOKEN` or FCM/APNs credentials in Expo are wrong or expired.
3. `MessageRateExceeded`: we are sending too fast; lengthen
   `NOTIFIER_INTERVAL` temporarily.
4. Undelivered alerts are retried on the next notifier run, because a
   subscription is only marked notified after at least one delivery succeeds.

### AuraRiskNotifierStalled

Covers `AuraRiskNotifierStalled` (no successful run in 3× `NOTIFIER_INTERVAL`)
and `AuraRiskNotifierFailing` (every run in the last hour failed). No
subscriptions are being evaluated, so **no flood alerts are going out**.

1. Backend logs: `❌ Notifier run failed: ...` gives the reason, usually the
   database.
2. If no instance logs runs at all, check that `NOTIFIER_INTERVAL` is not `0`
   and that at least one backend instance is up.
3. Runs coordinate with a Postgres advisory lock (`pg_try_advisory_lock(7311002)`).
   A hung session holding it blocks every instance:
   `SELECT pid, state, query_start FROM pg_stat_activity WHERE pid IN (SELECT pid FROM pg_locks WHERE locktype = 'advisory' AND objid = 7311002);`
   Terminate it if it is stuck.

### AuraRiskPhotoProviderFailing

Covers S3 and ClamAV failures (`AuraRiskPhotoProviderFailing`) and
`AuraRiskPhotoProcessorStalled`. Photo processing retries up to 5 times, then
marks the photo failed; reports themselves are unaffected.

1. ClamAV: check that clamd is running and reachable at `CLAMD_ADDR`. After a
   restart it takes 1–3 minutes to load signatures.
2. S3: check credentials, bucket permissions, and the provider's status page.
3. Processor stalled: the backend cannot read the photo queue. Check database
   health and the processor's logs (`❌ Photo claim failed`).

### AuraRiskReportCuration

Covers `AuraRiskReportCurationStalled` (the report verifier or retention worker
has not completed a run in 3× its interval) and `AuraRiskModerationBacklog`
(more than 200 pending reports for 6 hours). See
[REPORT_CURATION.md](REPORT_CURATION.md) for how both work.

1. Stalled worker: backend logs show `❌ Report verification failed` or
   `❌ Retention cleanup failed` with the cause, usually the database. Check
   that `REPORT_VERIFIER_INTERVAL` / `RETENTION_INTERVAL` are not `0`.
2. Retention running but not deleting: `⚠️ Retention could not delete object`
   means object storage is failing; see
   [AuraRiskPhotoProviderFailing](#aurariskphotoproviderfailing). Reports are
   kept until their photos can be removed, so nothing is orphaned.
3. Backlog: tell the moderators. A sudden jump with many similar notes is
   likely spam; reject a sample and check whether the duplicate rules caught
   the rest (`GET /api/moderation/reports?duplicates=only`).
