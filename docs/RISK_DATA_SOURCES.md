# Risk model data sources and calibration plan

How to give the flood risk score real inputs and validate it before launch
([#22](https://github.com/iochieng1/Aura-Risk/issues/22)). Until this is done
the score is a heuristic: the confidence level is capped at *medium*
(`modelCalibrated` in `internal/services/confidence.go`), and every client shows
the decision-support notice.

## What the score uses today

`calculateRiskScore` in `aurarisk-backend/internal/services/risk.go` adds up:

| Input | Source | Status |
|---|---|---|
| Rain in the next hours | Open-Meteo forecast `precipitation` | Real data |
| Rain in the last 48 h and 7 days | Open-Meteo `past_days` | Real data |
| Topsoil moisture (0–7 cm) | Open-Meteo `soil_moisture_0_to_7cm` | Real data |
| Thunderstorm / heavy-shower weather code | Open-Meteo `current.weather_code` | Real data |
| **Terrain factor (up to 20 points)** | `abs(sin(lat*100) * cos(lon*100)) * 20` | **Placeholder. Not related to the ground at all** |

The terrain factor looks random from place to place, and it can add up to a
fifth of the score. Replacing it is the single most valuable change.

All weights and thresholds (e.g. `antecedent48h/50*20`) are hand-picked and
have never been compared with observed floods.

## Staying on Open-Meteo

There's no need to change weather providers. Open-Meteo has more free datasets
than the forecast endpoint AuraRisk calls now:

| Dataset | Open-Meteo API | Use in AuraRisk |
|---|---|---|
| Elevation | Elevation API (`/v1/elevation`, Copernicus DEM) | Replace the terrain placeholder. Elevation never changes, so cache it per grid cell for good. |
| River discharge | Flood API (GloFAS river discharge, past and forecast) | A direct flood signal the score doesn't use yet. Best for larger rivers; small urban streams are not resolved. |
| Rainfall history | Historical Weather API (ERA5 reanalysis, decades of data) | Replay past storms through the score to calibrate it (see below). |
| Deeper soil moisture | Same forecast API, more `soil_moisture_*` layers | Better antecedent wetness than the top 7 cm alone. |

Each is a small extension of `internal/services/openmeteo.go`, and fits the
existing caching, retries, circuit breaker and stale-data policy.

### Licence and limits

- The **free API is for non-commercial use only** (already noted in
  `.env.example`). If AuraRisk earns money or is run by an organization, choose
  one of the two options below, and check Open-Meteo's current terms first:
  - **Paid plan:** set `OPEN_METEO_API_KEY` and the backend switches to
    `customer-api.open-meteo.com`.
  - **Self-host:** Open-Meteo is open source and can run in your own
    container. Point `OPEN_METEO_URL` at it. This removes the API rate limits,
    but you run its data downloads and storage yourself.
- Open-Meteo data must be attributed (CC BY 4.0). Add the attribution to the UI
  before launch.

## Data that doesn't change per request

Terrain, drainage, soil and land cover change slowly, so don't fetch them on
each request. Precompute them per area into PostGIS (see
[#18](https://github.com/iochieng1/Aura-Risk/issues/18)) and look them up by
location.

| Factor | Free source | What to derive |
|---|---|---|
| Drainage / low-lying ground | MERIT Hydro, HydroSHEDS | Height above the nearest drainage line, distance to a river, upstream catchment area |
| Elevation (finer than Open-Meteo's) | Copernicus DEM GLO-30 | Local slope, depressions where water collects |
| Soil type | SoilGrids (ISRIC) | How fast water soaks in (clay drains slowly, sand fast) |
| Land cover | ESA WorldCover | Built-up (paved) share, which increases runoff |

## Ground truth for validation

Validation needs to know where and when it actually flooded.

| Source | Coverage |
|---|---|
| Global Flood Database (satellite-mapped flood extents) | Worldwide, historical events |
| Copernicus Emergency Management Service rapid mapping | Major disasters worldwide |
| Dartmouth Flood Observatory, EM-DAT | Event lists with dates and areas |
| **Kenya Red Cross** situation reports and **ReliefWeb** (OCHA) flash updates | Kenya; dated, named places for each flood event, e.g. the March–May 2024 long rains |
| **Kenya Meteorological Department** (KMD) forecasts and advisories | Kenya; the official warnings to compare the score with |
| **NDMA** early warning bulletins | Kenya, monthly per county; drought-focused, with flood notes |
| **Water Resources Authority** river gauges | Kenya; gauge records for rivers such as the Nzoia and Nyando, on request |
| **County disaster management offices** | Kenya; local incident logs, e.g. Kisumu County for the Kano plains |
| **UP NOAH** flood hazard maps (5-, 25- and 100-year return periods) | Philippines |
| **HazardHunterPH / GeoRiskPH** (DOST-PHIVOLCS) | Philippines hazard layers |
| PAGASA rainfall and river bulletins | Philippines; check what is available as data |
| AuraRisk's own verified community reports | Grows over time; biased toward populated areas |

Kenya is the launch region, starting with Kisumu
([#56](https://github.com/iochieng1/Aura-Risk/issues/56)), so its sources come
first. The Philippine sources stay for a later region.

## Calibration plan

1. **Replace the terrain placeholder** with elevation and height above the
   nearest drainage line, and add GloFAS river discharge as an input. Bump
   `model_version` (e.g. `heuristic-2`).
2. **Build a backtest.** An offline script (not in the API path) that:
   - picks past flood events and an equal number of dry and wet non-flood days
     for the same places;
   - fetches ERA5 rainfall and river discharge for those dates;
   - runs the same scoring code over them and records the level it would
     have shown.
3. **Measure, per region:**
   - **Detection rate:** how often an Alert or Emergency came before a real flood.
   - **False alarm rate:** how often there was an Alert with no flood.
   - **Lead time:** how many hours before the flood the alert fired.
4. **Tune the weights and level thresholds** to the trade-off you choose (for
   flood warnings, missing a flood is usually worse than a false alarm). Keep a
   held-out set of events that are never used for tuning.
5. **Publish a model report for each launch region**: data used, metrics,
   known blind spots (small streams, drainage blockages, coastal surge), and
   the review by a domain expert. This is the #22 release gate.
6. **Set `modelCalibrated = true`** only for regions with a published report,
   so confidence can reach *high* there.

## Suggested order

1. Elevation and river discharge from Open-Meteo (small backend change).
2. PostGIS ([#18](https://github.com/iochieng1/Aura-Risk/issues/18)), then
   import drainage, soil and land cover layers.
3. Backtest script and the first regional model report.
