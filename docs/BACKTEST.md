# Backtesting the risk score

The backtest replays past weather and river discharge through the live
scoring code (`services.CalculateRiskScore`) and measures how the score would
have done on known flood days and on control days. It is step 3 of the
calibration plan in [RISK_DATA_SOURCES.md](RISK_DATA_SOURCES.md)
([#22](https://github.com/iochieng1/Aura-Risk/issues/22),
[#58](https://github.com/iochieng1/Aura-Risk/issues/58)). It runs offline and
is never part of the API.

## Running it

```sh
cd aurarisk-backend
go run ./cmd/backtest -events ../docs/backtest/kenya-2024-long-rains.csv \
  -json ../docs/backtest/kenya-2024-long-rains.heuristic-2.json
```

| Flag | Default | Meaning |
|---|---|---|
| `-events` | required | Events CSV (format below) |
| `-json` | none | Also write every outcome and the metrics as JSON |
| `-lead-days` | 3 | Days before the event day that are scored, besides the day itself |
| `-threshold` | `Alert` | Level that counts as a warning |
| `-delay` | 2s | Pause between events, to stay within Open-Meteo's free limits |

`OPEN_METEO_API_KEY`, `OPEN_METEO_FLOOD_URL` and `OPEN_METEO_ARCHIVE_URL` are
read as in the API. A run of 40 events makes about 120 Open-Meteo calls and
takes a few minutes.

## How an event is scored

For each event, every hour from `lead-days` before the event day to the end
of that day is scored as if it were "now":

- **Weather:** ERA5 reanalysis from the Historical Weather API, with the
  same 7-day history and 6-hour look-ahead as the live score.
- **River:** GloFAS discharge from the Flood API, with the same 92-day
  baseline, 3-day peak and main-river search.
- **Ground:** today's elevation (it doesn't change).

An event is **warned** if any scored hour reaches the threshold. For a flood,
the **lead time** is the number of hours from the first warning to the start
of the flood day.

**This is an upper bound.** Both the look-ahead rain and the river "forecast"
are what actually happened, not what was forecast at the time. Real
forecasts are worse, especially for convective rain.

## Events file

```csv
region,place,lat,lon,date,flooded,source
kisumu,Ahero,-0.1725,34.9190,2024-05-04,yes,https://citizen.digital/...
kisumu,Ahero,-0.1725,34.9190,2024-07-17,no,"control: dry season, no reports found"
```

- `date` is the local day it flooded (or the control day), `YYYY-MM-DD`.
- `flooded` is `yes` or `no`.
- Flood rows must cite a source URL that names the place and the day. Rows
  without one are rejected.
- Coordinates come from OpenStreetMap for the named place.

## Kenya, 2024 long rains (heuristic-2)

[`backtest/kenya-2024-long-rains.csv`](backtest/kenya-2024-long-rains.csv):
20 flood days from March–May 2024 news reports (Citizen Digital, The Star,
The Standard, Kenyans.co.ke, K24, Eastleigh Voice, Africanews, Nation via
mwananchi.co.tz), and one July or August 2024 control day per place. Results
are in
[`backtest/kenya-2024-long-rains.heuristic-2.json`](backtest/kenya-2024-long-rains.heuristic-2.json).

| Region | Floods | Detected | Controls | False alarms | Median lead |
|---|---|---|---|---|---|
| Kisumu | 4 | 4 | 4 | 2 | 58 h |
| Nairobi | 7 | 7 | 7 | 0 | 34 h |
| Tana / Garissa | 4 | 4 | 4 | 0 | 72 h |
| Busia (Budalang'i) | 1 | 1 | 1 | 1 | 43 h |
| Other | 4 | 4 | 4 | 1 | 72 h |
| **All** | **20** | **20 (100%)** | **20** | **4 (20%)** | **61 h** |

### What this does and doesn't show

- **The score is high throughout the long rains, not just on flood days.**
  Every flood was warned, but in several regions the first warning came at
  the very first scored hour, so the 72-hour lead time is the window's limit
  rather than real warning skill. The controls are July and August days,
  outside the rainy season, so they only test that the score is low when it
  is dry. That isn't a hard test.
- **The western controls weren't dry.** Ayweyo and Ombeyi (Kisumu) and
  Mabinju (Budalang'i) reached Emergency in August. The Lake Victoria basin
  gets rain in July and August, and Budalang'i stayed flooded for months in
  2024. These may be real wet periods without a reported flood, or real
  floods nobody reported. Either way, "dry season" was the wrong assumption
  there.
- **Too few events.** 20 floods, Kisumu has 4 and Budalang'i 1. No numbers
  here are reliable per region.

### Next steps

1. **Add wet non-flood days.** Use days in March–May 2024 at the same places
   when no flooding was reported, for example a week before or after each
   event. This is the comparison that tests whether the score separates
   flood days from rainy days. Without it, weights can't be tuned.
2. **More Kisumu and Budalang'i events** from Kenya Red Cross and county
   reports, and the 2023 short rains (October–December).
3. **Score real forecasts.** Open-Meteo's Historical Forecast API keeps past
   forecasts. Using it would remove the perfect-forecast assumption.
4. Only then tune the weights and thresholds (step 4 of the calibration
   plan), keeping a held-out set of events.
