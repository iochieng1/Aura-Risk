# Mobile Phase 3: Notifications, Background Refresh, Accounts, Photos

This document is the API contract between `aurarisk-backend` and `aurarisk-mobile`
for Phase 3 of mobile delivery.

## Accounts and tokens

The app uses anonymous device accounts. On first launch the app registers
itself and receives a short-lived access token and a long-lived refresh token.
The server stores only SHA-256 hashes of tokens. The app stores tokens only in
the platform keystore via `expo-secure-store` (never AsyncStorage).

- Access token TTL: 15 minutes. Sent as `Authorization: Bearer <access_token>`.
- Refresh token TTL: 30 days. Rotated on every refresh. Presenting a refresh
  token that was already rotated revokes every token for that device (reuse
  detection), and the app must re-register.

| Method | Path | Auth | Body | Response |
|---|---|---|---|---|
| POST | `/api/auth/device` | none | `{platform: "ios"\|"android"\|"web", app_version?: string}` | `201 TokenPair` |
| POST | `/api/auth/refresh` | none | `{refresh_token}` | `200 TokenPair` / `401` |
| POST | `/api/auth/logout` | bearer | - | `204` (revokes this device's tokens and push token) |
| DELETE | `/api/account` | bearer | - | `204` (deletes account, devices, subscriptions; detaches reports) |

```ts
type TokenPair = {
  account_id: string;
  device_id: string;
  access_token: string;
  access_expires_at: string;  // RFC 3339
  refresh_token: string;
  refresh_expires_at: string; // RFC 3339
};
```

## Push notifications

Push is opt-in. The app shows its own explanation first and only requests the
OS permission after the user agrees. The backend stores a push token only when
the request carries `consent: true`, and records when consent was given.
Delivery goes through the Expo Push Service on the Android channel
`risk-alerts`. Tickets reporting `DeviceNotRegistered` clear the stored token.

| Method | Path | Auth | Body | Response |
|---|---|---|---|---|
| PUT | `/api/devices/me/push` | bearer | `{push_token: "ExponentPushToken[...]", consent: true}` | `200 {push_enabled: true, consented_at}` |
| DELETE | `/api/devices/me/push` | bearer | - | `204` (withdraws consent, clears token) |

## Location subscriptions

| Method | Path | Auth | Body | Response |
|---|---|---|---|---|
| GET | `/api/subscriptions` | bearer | - | `200 Subscription[]` |
| POST | `/api/subscriptions` | bearer | `SubscriptionInput` | `201 Subscription` (max 10 per account) |
| PUT | `/api/subscriptions/:id` | bearer | `SubscriptionInput` | `200 Subscription` |
| DELETE | `/api/subscriptions/:id` | bearer | - | `204` |

```ts
type RiskLevel = 'Advisory' | 'Alert' | 'Emergency';

type SubscriptionInput = {
  name: string;                 // 1..100 chars
  lat: number;                  // -90..90
  lon: number;                  // -180..180
  min_level: RiskLevel;         // notify at this level or higher
  quiet_hours: {
    start: string;              // "HH:MM", local to timezone
    end: string;                // "HH:MM"; may be earlier than start (overnight)
    timezone: string;           // IANA, e.g. "Africa/Nairobi"
  } | null;
  allow_emergency_during_quiet_hours: boolean;
};

type Subscription = SubscriptionInput & {
  id: string;
  last_notified_level: string | null;
  last_notified_at: string | null;
  created_at: string;
};
```

The notifier runs on the server every `NOTIFIER_INTERVAL` (default 15m). For
each subscription it computes risk, then sends a push when:

1. the level is at or above `min_level`,
2. the level is higher than `last_notified_level`, or the last notification is
   older than 12 hours, and
3. the current time is outside quiet hours, unless the level is `Emergency`
   and `allow_emergency_during_quiet_hours` is true.

When the level falls below `min_level`, `last_notified_level` resets so a new
escalation notifies again. Push `data` payload:
`{type: "risk_alert", subscription_id, level, score, lat, lon}`.

## Photo attachments

Photos never go through the database as blobs. The flow:

1. The app creates the report (`POST /api/reports` with a bearer token so the
   report is owned by the account).
2. `POST /api/reports/:id/photos` with `{content_type, size_bytes}` returns a
   presigned `PUT` URL to the **quarantine** prefix in S3. Content-Type and
   Content-Length are part of the signature.
3. The app uploads the bytes directly to S3 with the returned headers.
4. `POST /api/photos/:id/complete` marks the upload done.
5. A server worker downloads the object, checks size and magic bytes, scans it
   with ClamAV (`clamd` INSTREAM), decodes it, and re-encodes it as JPEG at two
   sizes (`large` 1600px, `thumb` 400px). Re-encoding strips EXIF, including
   GPS. Variants are written to the `photos/` prefix and the quarantine object
   is deleted. Infected or undecodable files are marked `rejected`.
6. Ready photos are served through short-lived presigned GET URLs.

If no scanner is configured, photos stay unpublished (fail closed).

| Method | Path | Auth | Body | Response |
|---|---|---|---|---|
| POST | `/api/reports/:id/photos` | bearer, report owner | `{content_type: "image/jpeg"\|"image/png"\|"image/webp", size_bytes: number}` | `201 PhotoUpload` |
| POST | `/api/photos/:id/complete` | bearer, owner | - | `202 Photo` |
| GET | `/api/photos/:id` | bearer, owner | - | `200 Photo` |

Limits: 10 MB per photo, 4 photos per report, upload URL valid for 15 minutes.
Slots that were never completed stop counting toward the limit when their URL
expires, and completing an expired slot returns `410`; the client requests a
new slot and uploads again.

```ts
type PhotoUpload = {
  photo_id: string;
  upload_url: string;
  upload_method: 'PUT';
  upload_headers: Record<string, string>;
  expires_at: string;
};

type Photo = {
  id: string;
  status: 'pending_upload' | 'processing' | 'ready' | 'rejected' | 'failed';
  thumb_url?: string;  // only when ready
  large_url?: string;  // only when ready
};
```

`GET /api/reports` includes `photos: [{id, thumb_url, large_url}]` for ready
photos on each report.

## Mobile background refresh

The app registers an `expo-background-task` task. The OS decides when it runs
(minimum interval 15 minutes, subject to battery budget, Low Power Mode, and
app standby buckets). Each run is short and does two things: it flushes the
offline report queue and refreshes cached risk for subscribed locations. It
skips network work when Low Power Mode is on.

## Local development

```bash
docker compose up -d postgres objectstore objectstore-init clamav
cp .env.example aurarisk-backend/.env
cd aurarisk-backend && go run .
```

`objectstore` is RustFS, an S3-compatible server, and `objectstore-init`
creates the `aurarisk-photos` bucket. ClamAV downloads signatures on first
start, so photos stay `processing` for a few minutes until `clamd` is healthy.
To test on a phone, set `S3_PUBLIC_ENDPOINT` and `EXPO_PUBLIC_API_BASE_URL` to
your machine's LAN address. Push notifications need an EAS project ID
(`npx eas-cli@latest init`) and a development build; they don't work in Expo Go.

Run the DB-backed notifier test against a disposable database:

```bash
TEST_DATABASE_URL=postgres://... go test ./internal/workers/
```

## Production notes

- Add an S3 lifecycle rule that expires `quarantine/` objects after 1 day as a
  backstop for abandoned uploads.
- Keep the bucket private; photos are only served through presigned URLs.
- The notifier reads Expo push tickets but not receipts yet. Polling receipts
  would catch tokens that fail after delivery is accepted.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `S3_BUCKET` | - | Bucket for photos (required for photo endpoints) |
| `S3_REGION` | `us-east-1` | |
| `S3_ENDPOINT` | AWS | Internal endpoint, e.g. `http://localhost:9000` for MinIO |
| `S3_PUBLIC_ENDPOINT` | `S3_ENDPOINT` | Endpoint used in presigned URLs, must be reachable from phones |
| `S3_FORCE_PATH_STYLE` | `false` | `true` for MinIO |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | - | Standard AWS credential chain |
| `CLAMD_ADDR` | - | `host:port` of clamd; unset means photos are never published |
| `EXPO_PUSH_URL` | `https://exp.host/--/api/v2/push/send` | |
| `EXPO_ACCESS_TOKEN` | - | Optional Expo push security token |
| `NOTIFIER_INTERVAL` | `15m` | `0` disables the notifier |
