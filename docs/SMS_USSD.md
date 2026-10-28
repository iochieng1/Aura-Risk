# SMS and USSD access

Many people in the pilot areas use feature phones. Through
[Africa's Talking](https://africastalking.com), anyone can check flood risk and
report flooding by USSD or SMS, without a smartphone or mobile data (#61).

## What people can do

**USSD** (e.g. dial `*384*1234#`), in English or Kiswahili:

```
Welcome to AuraRisk / Karibu AuraRisk     1. Check flood risk
1. English                          ->    2. Report flooding
2. Kiswahili
```

- **Check flood risk:** enter a county. The answer is the risk level and score
  at the county headquarters, the first safety tip, and a reminder that this is
  decision support only.
- **Report flooding:** enter the county, then the ward (or `0` to skip), then
  choose what is happening (flooding, road blocked, water rising, drainage
  blocked).

**SMS** to the short code:

| Message | Reply |
|---|---|
| `RISK Kisumu` / `HATARI Kisumu` | Flood risk at Kisumu's headquarters |
| `FLOOD Kisumu, Nyalenda A, water over the road` | Files a flooding report. The parts are county, ward and what you see |
| `MAFURIKO Kisumu maji yamejaa barabarani` | Without commas, the leading words that name a county are the county and the rest is the note |
| `ROAD` / `BARABARA`, `RISING` / `MAJI`, `DRAIN` / `MTARO` | The same, for the other report categories |
| Anything else, `HELP`, `MSAADA` | How to use the service |

Replies are in the language of the keyword.

## How channel reports are handled

- They go through the same validation, duplicate detection and moderation as
  app reports, and start as pending.
- **Location:** a feature phone doesn't send its position, so reports are
  placed at the county headquarters (`services.CountyHeadquarters`), with the
  ward in the place name. They are stored with `source` = `sms` or `ussd` and
  `location_approximate = true`, and public responses include
  `"location_approximate": true`.
  - Because they all share a point, approximate reports **never corroborate**
    other reports in automatic verification. Moderators verify them.
  - Risk checks are for the headquarters town, not the whole county.
- **Phone numbers** are never stored (Data Protection Act 2019). Each number is
  normalized (`0712…`, `+254712…` and `254712…` are the same) and stored only as
  `HMAC-SHA256(CHANNEL_PHONE_SALT, number)` in `channel_reporters`, linked to
  an account. That account gives channel reporters the same duplicate
  detection and reputation as app users. Deleting the account deletes the
  mapping. Keep the salt secret: with it, a number can be checked against the
  hash.
- **Limits:** `CHANNEL_RATE_LIMIT_PER_MINUTE` (10) requests per phone across
  both channels, and `CHANNEL_REPORTS_PER_HOUR` (5) reports per phone. The
  API's per-IP limit doesn't apply, because every request comes from Africa's
  Talking.

## Setup

1. Create an Africa's Talking account. Start in the **sandbox** (free, with a
   simulator for USSD and SMS) before going live.
2. Generate secrets:

   ```sh
   CHANNEL_CALLBACK_SECRET=$(openssl rand -hex 32)
   CHANNEL_PHONE_SALT=$(openssl rand -hex 32)
   ```

3. Configure the API:

   | Variable | Purpose |
   |---|---|
   | `CHANNEL_CALLBACK_SECRET` | Enables both callbacks. It must match the `key` query parameter of the callback URLs. 32+ characters in production |
   | `CHANNEL_PHONE_SALT` | Salt for the phone hash. 32+ characters in production. **Never change it** once live, or every reporter gets a new account |
   | `AT_USERNAME`, `AT_API_KEY` | For sending SMS replies (`sandbox` is the sandbox username). Without them, replies are logged, not sent |
   | `AT_SMS_FROM` | Short code or sender ID to reply from |
   | `AT_SANDBOX=true` | Send through the sandbox API |

   Without `CHANNEL_CALLBACK_SECRET`, both endpoints answer 404.
4. In the Africa's Talking dashboard, set the callback URLs:
   - USSD: `https://<api host>/api/channels/ussd?key=<CHANNEL_CALLBACK_SECRET>`
   - SMS inbound: `https://<api host>/api/channels/sms?key=<CHANNEL_CALLBACK_SECRET>`

   Africa's Talking can't sign callbacks, so the secret in the URL is what
   authenticates them. Keep it out of logs and screenshots, and rotate it by
   changing both the variable and the dashboard.

## Short code and USSD code

- **USSD:** for a pilot, rent a channel on Africa's Talking's **shared** code
  (`*384*NNNN#`). A **dedicated** code (e.g. `*123#`) needs approval from the
  Communications Authority of Kenya and agreements with the mobile networks,
  which takes weeks.
- **SMS:** a **shared short code** with a keyword works for a pilot. A
  **dedicated short code** has to be set up with each network.
- Decide whether users pay for SMS (standard rate) or AuraRisk does
  (zero-rated / reverse-billed, which costs more). Zero-rating makes the
  service usable by people with no airtime.

## Costs

Africa's Talking bills per use; check its
[pricing page](https://africastalking.com/pricing) for current Kenyan rates.
The cost lines to budget:

- **USSD:** a monthly fee for the shared-code channel (or setup plus monthly
  fees for a dedicated code), plus a charge per session.
- **SMS:** a charge per outgoing reply, which every inbound SMS triggers.
  Inbound SMS cost the sender their normal rate unless the code is zero-rated.
- **Short code:** setup and monthly fees for a dedicated code.

Sessions and replies both scale with use, so the rate limits above also cap
cost per phone. Watch for spikes in the `aurarisk_rate_limited_total{limiter="channels"}`
metric.
