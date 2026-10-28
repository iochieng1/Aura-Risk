# County response

Verified community reports are routed to the county government responsible,
and residents can see what happened to each one (#62). This builds on the
county and ward that reporters give (#59).

## Response status

Every report has a **response status**, separate from its moderation status:

| Status | Meaning |
|---|---|
| `open` | Not yet picked up by the county (the default) |
| `acknowledged` | A county responder has seen it and is acting |
| `resolved` | The county has dealt with it |

- Only **verified, non-duplicate reports with a county** reach responders.
- Moves only go forward: `open → acknowledged → resolved`. Resolving an open
  report directly also records it as acknowledged at that moment. Any other
  move gets **409**.
- Each move is stored in `report_response_events` (who, from, to, optional
  note, when), and the report keeps `acknowledged_at`, `resolved_at` and
  `response_updated_at`. The county dashboard (#63) uses these timestamps.

Public responses (`GET /api/reports`, `POST /api/reports`) include
`response_status` and, once it has changed, `response_updated_at`. The web and
mobile apps show it next to each verified report, e.g. "Verified · County
acknowledged".

## Responder API

Responders authenticate with a bearer token from `RESPONDER_TOKENS`, a
comma-separated list of `name:County:token`:

```sh
RESPONDER_TOKENS="kisumu-desk:Kisumu:$(openssl rand -hex 32),homabay-desk:Homa Bay:$(openssl rand -hex 32)"
```

The county is matched like report counties (case and a trailing "County" are
ignored); startup fails on an unknown county. Production requires tokens of
32+ characters. The name is recorded in the audit trail. Unset disables the
API (503).

Each responder only sees their own county. Reports in other counties, and
reports that aren't verified, answer **404**.

| Endpoint | Purpose |
|---|---|
| `GET /api/responder/reports?status=open\|acknowledged\|resolved&limit=1..100` | The county's verified reports with that status (default `open`), newest first, with photos |
| `POST /api/responder/reports/:id` `{"status": "acknowledged"\|"resolved", "note": "..."}` | Move a report forward. `note` is optional, up to 500 bytes, and only stored in the audit trail |
| `GET /api/responder/reports/:id/events` | The report's response history, oldest first |

## County webhooks

When a report with a county is verified (by a moderator or automatically),
AuraRisk can POST it to that county's system: a WhatsApp or SMS gateway, a
ticketing tool, or a shared inbox bridge.

```sh
COUNTY_WEBHOOKS="Kisumu=https://hooks.kisumu.go.ke/aurarisk,Homa Bay=https://example.org/hb"
COUNTY_WEBHOOK_SECRET=$(openssl rand -hex 32)
COUNTY_WEBHOOK_INTERVAL=30s   # how often the worker runs
```

- Production requires `https` URLs and a secret of 32+ characters.
- Deliveries go through the `report_webhook_deliveries` outbox, so each report
  is sent once even with several API instances. A non-2xx answer or a network
  error is retried with backoff (1 minute, doubling, at most 6 hours apart),
  up to 10 attempts. `last_error` keeps the latest failure.
- Only reports verified in the last 24 hours are queued, so adding a county's
  webhook doesn't replay its history.

Request:

```http
POST <county URL>
Content-Type: application/json
X-AuraRisk-Event: report.verified
X-AuraRisk-Delivery: <report id>
X-AuraRisk-Timestamp: <unix seconds>
X-AuraRisk-Signature: sha256=<hex HMAC-SHA256(secret, timestamp + "." + body)>

{
  "event": "report.verified",
  "report": {
    "id": "…", "location": {"name": "…", "lat": -0.1, "lon": 34.75},
    "category": "flooding", "note": "…", "timestamp": "…",
    "status": "verified", "corroborations": 2,
    "county": "Kisumu", "ward": "Nyalenda A",
    "response_status": "open", "photos": []
  },
  "sent_at": "…"
}
```

To check a delivery, recompute the signature over the raw body with the shared
secret, compare it in constant time, and reject timestamps more than about
5 minutes old. Use `X-AuraRisk-Delivery` to ignore repeats: a retry after a
lost response sends the same report again.

## Operating it

- Agree with each county who holds the responder token, and rotate tokens by
  changing `RESPONDER_TOKENS` and redeploying.
- Watch `aurarisk_worker_runs_total{worker="county_webhooks"}` for failures,
  and look for rows with `delivered_at IS NULL AND attempts >= 10` (given up).
