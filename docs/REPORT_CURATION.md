# Report curation

Community reports are moderated, checked for duplicates, verified
automatically when there is enough evidence, and deleted on a schedule.

## Moderation status

Every report has a `status`:

| Status | Meaning | Public? |
|---|---|---|
| `pending` | Not yet verified. All new reports start here. | Yes, shown as unverified |
| `verified` | Corroborated automatically or confirmed by a moderator | Yes |
| `rejected` | A moderator decided it is false, spam, or abusive | No |

Pending reports stay public because a flood warning that waits for review
helps no one. Clients should label them as unverified. Clients that only want
confirmed reports can pass `verified_only=true` to `GET /api/reports`.

Moderator decisions are final: automatic verification never changes a report a
moderator has verified or rejected. A moderator can set a report back to
`pending` to hand it back to automatic verification.

Every status change, automatic or manual, is recorded in
`report_moderation_events` with who made it and why.

## Duplicate detection

Runs when a report is created (`internal/services/curation.go`). A new report
is a duplicate when either:

- **The same account** reported the same kind of problem within **500 m** and
  **3 hours**. `flooding` and `water_rising` count as the same kind.
- **Anyone** posted near-identical text within **250 m** and **24 hours**:
  word-set similarity of at least 0.8, or identical text for notes under three
  words. This catches copy-paste from anonymous clients.

A duplicate is still saved, with `duplicate_of` set to the original report.
`POST /api/reports` returns it with `duplicate_of`, and listings hide it.
Duplicates of duplicates point to the original. Rejected reports are never
used as originals.

Different people describing the same flood in their own words are **not**
duplicates. Those reports corroborate each other (see below).

## Automatic verification

A worker (`REPORT_VERIFIER_INTERVAL`, default 2 minutes) rescores pending
reports from the last 48 hours. The score runs from 0 to 100:

| Evidence | Points |
|---|---|
| Each other signed-in account reporting the same kind of problem within 1 km and ±6 hours | +25 |
| Any anonymous corroborating report (counted once, since they may all be one person) | +15 |
| Corroboration subtotal | capped at 50 |
| A photo that passed malware scanning and processing | +20 |
| Filed from a signed-in device | +10 |
| Each of the reporter's past reports **verified by a moderator** | +10 (max +20) |
| Each of the reporter's past reports **rejected by a moderator** | −25 |

At **50 or more** the report is verified, with `moderated_by = "auto"`. In
practice that means:

- two independent people reporting the same thing, or
- one corroborating report plus a photo from a signed-in device, or
- a reporter with a good moderation record who includes a photo.

Automatic verification never rejects. Only moderator decisions affect a
reporter's track record, so auto-verified reports cannot inflate it. The score
and corroboration count are stored on each report, and the public API returns
`corroborations`.

## Moderation API

Moderators authenticate with a bearer token from `MODERATOR_TOKENS`
(`name:token,name:token`). The name is recorded in the audit trail. Without
any tokens the API returns `503`. Tokens can come from a file
(`MODERATOR_TOKENS_FILE`) or AWS Secrets Manager like the other secrets, and
production requires at least 32 characters each.

| Method | Path | Body / query | Result |
|---|---|---|---|
| GET | `/api/moderation/reports` | `status` (pending\|verified\|rejected, default pending), `duplicates` (exclude\|include\|only, default exclude), `before` (RFC 3339 cursor), `limit` (1–100) | Reports newest first, with score, reporter history, and decision |
| POST | `/api/moderation/reports/:id` | `{"status": "verified" \| "rejected" \| "pending", "reason": "...", "not_duplicate": false}` | The updated report. `reason` is required to reject. `not_duplicate: true` clears a wrong duplicate match. |
| GET | `/api/moderation/reports/:id/events` | — | The report's moderation history |

Example:

```bash
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/moderation/reports?status=pending"
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"rejected","reason":"advertising"}' \
  http://localhost:8080/api/moderation/reports/<id>
```

## Retention

A worker (`RETENTION_INTERVAL`, default hourly) deletes expired data. Each
period is configurable in days, and `0` keeps that data forever.

| Data | Kept for | Measured from | Setting |
|---|---|---|---|
| Rejected reports | 30 days | rejection | `RETENTION_REJECTED_DAYS` |
| Duplicate reports | 30 days | creation | `RETENTION_DUPLICATE_DAYS` |
| Pending reports never verified or rejected | 180 days | creation | `RETENTION_PENDING_DAYS` |
| Verified reports | 2 years | creation | `RETENTION_VERIFIED_DAYS` |
| Photos that failed processing or were rejected | 30 days | last update | `RETENTION_FAILED_PHOTO_DAYS` |
| Photo uploads never completed | 1 day | creation | fixed |

Deleting a report also deletes its photos, its moderation history, and any
duplicates of it. Photo objects are removed from object storage **before** the
rows. If storage is unavailable, the report is kept and retried on the next
run, so files are never orphaned. When object storage is not configured,
reports with photos are skipped rather than leaving files behind.

Verified reports are kept the longest because they are the record of real
flood events, which the risk model needs for calibration.

The verifier and retention workers both use Postgres advisory locks, so
running several backend instances is safe.

## Monitoring

| Metric | Meaning |
|---|---|
| `aurarisk_moderation_queue_reports` | Pending, non-duplicate reports |
| `aurarisk_reports_auto_verified_total` | Reports verified automatically |
| `aurarisk_retention_deleted_total{kind,reason}` | Rows removed by retention |
| `aurarisk_worker_last_success_timestamp_seconds{worker="report_verifier"\|"retention"}` | Worker health |

The *Report curation* row on the Grafana dashboard shows these. Tickets fire
for a stalled worker and for a queue above 200 for 6 hours (see
[ON_CALL.md](ON_CALL.md#aurariskreportcuration)).
