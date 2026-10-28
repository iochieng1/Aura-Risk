# County dashboard and open data

Residents, journalists and researchers can see how each county responds to
flood reports, and other projects can reuse the data (#63). Both use only
**verified, non-duplicate** reports. Response statuses come from county
responders (see [COUNTY_RESPONSE.md](COUNTY_RESPONSE.md)).

The web app's **County dashboard** (`#/counties`) shows the statistics for the
last 30 days, 90 days or 12 months, with download links for the export.

## Periods

Both endpoints take `from` and `to` as `YYYY-MM-DD` (UTC, both inclusive) and
filter on when the report was **created**. The default is the 90 days up to
today. A period can be at most 366 days. Responses are cacheable for 5 minutes.

## `GET /api/stats/counties`

```json
{
  "from": "2026-08-01",
  "to": "2026-10-29",
  "counties": [
    {
      "county": "Kisumu",
      "verified": 42,
      "open": 10,
      "acknowledged": 12,
      "resolved": 20,
      "median_hours_to_acknowledge": 5.5,
      "median_hours_to_resolve": 30.2
    }
  ]
}
```

- Only counties with at least one verified report in the period are listed,
  most reports first.
- `open`, `acknowledged` and `resolved` count reports by their **current**
  response status, so they add up to `verified`.
- Medians are hours from the report's creation, to one decimal place, over the
  reports that reached that step. They are `null` when none did. A report
  resolved without a separate acknowledgement counts as acknowledged when it
  was resolved.

## `GET /api/open-data/reports.csv` and `.json`

Optional `county` (any of the 47, matched loosely). Rows are oldest first, up
to 50,000 per request; split longer periods.

### Data dictionary

| Column | Type | Meaning |
|---|---|---|
| `created_at` | RFC 3339, UTC | When the report was made |
| `category` | text | `flooding`, `road_blocked`, `water_rising` or `drainage_issue` |
| `county` | text | Official county name; empty when the reporter gave none |
| `ward` | text | Ward as typed by the reporter; empty when not given |
| `lat`, `lon` | number | Location rounded to 2 decimal places (about 1.1 km) |
| `corroborations` | integer | Independent nearby reports of the same kind of problem |
| `response_status` | text | `open`, `acknowledged` or `resolved` |
| `acknowledged_at`, `resolved_at` | RFC 3339, UTC | When the county acknowledged or resolved it; empty when it hasn't |

The JSON file wraps the same rows as `reports`, together with `license`,
`license_url`, `attribution`, `from`, `to` and `generated_at`. In JSON, empty
values are `null`.

In the CSV, a ward that starts with `=`, `+`, `-`, `@`, tab or carriage return
is prefixed with `'`, so spreadsheets show it as text rather than run it as a
formula.

### Licence

[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). Credit
"AuraRisk community flood reports". Each response also carries
`Link: <https://creativecommons.org/licenses/by/4.0/>; rel="license"`.

Reports are written by members of the public and verified by corroboration and
moderators, not by an official agency. They show where people reported
flooding, not everywhere it flooded: busy areas report more.

### How anonymisation works

The export is meant to be safe to republish in bulk:

- **Left out:** the note, photos, the place name typed by the reporter, the
  report ID, the account and device, and anything else about the reporter.
  These can identify people or homes, and the ID would link a row back to the
  note through the public API.
- **Coarsened:** coordinates are rounded to 0.01° (about 1.1 km at the
  equator), which is roughly ward scale.
- **Only verified reports**, never pending, rejected or duplicate ones.
- Small counts can still be revealing (for example, a single report in a
  remote ward). Consider this before publishing maps at ward level.
