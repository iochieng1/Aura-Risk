import type { Coordinates } from './types';

/** Free Open-Meteo API (non-commercial use only). */
export const OPEN_METEO_FORECAST_URL = 'https://api.open-meteo.com/v1/forecast';

/**
 * The forecast request the backend makes (fetchOnce in
 * aurarisk-backend/internal/services/openmeteo.go). Keep the two in sync: the
 * backend validates and scores this response in POST /api/risk.
 */
export function openMeteoForecastUrl({ lat, lon }: Coordinates): string {
  const query = new URLSearchParams({
    latitude: lat.toFixed(6),
    longitude: lon.toFixed(6),
    past_days: '7',
    forecast_days: '2',
    current: 'temperature_2m,relative_humidity_2m,precipitation,rain,weather_code',
    hourly: 'precipitation,rain,soil_moisture_0_to_7cm',
    timezone: 'auto',
  });
  return `${OPEN_METEO_FORECAST_URL}?${query}`;
}

export const OPEN_METEO_ELEVATION_URL = 'https://api.open-meteo.com/v1/elevation';
export const OPEN_METEO_FLOOD_URL = 'https://flood-api.open-meteo.com/v1/flood';

/**
 * The point followed by the eight points `step` degrees around it, as the
 * backend's neighbourhood() in internal/services/hydrology.go orders them.
 */
function neighbourhood({ lat, lon }: Coordinates, step: number) {
  const points = [[lat, lon]];
  for (const dy of [-1, 0, 1]) {
    for (const dx of [-1, 0, 1]) {
      if (dy !== 0 || dx !== 0) points.push([lat + dy * step, lon + dx * step]);
    }
  }
  return {
    latitude: points.map(([la]) => la.toFixed(4)).join(','),
    longitude: points.map(([, lo]) => lo.toFixed(4)).join(','),
  };
}

/**
 * Elevation of the point and its neighbours ~1 km away (FetchTerrain in
 * hydrology.go). Sent with device weather to POST /api/risk (#69).
 */
export function openMeteoElevationUrl(at: Coordinates): string {
  return `${OPEN_METEO_ELEVATION_URL}?${new URLSearchParams(neighbourhood(at, 0.01))}`;
}

/**
 * GloFAS river discharge for the point's ~5 km cell and its neighbours, 92
 * days back and 7 ahead (FetchRiver in hydrology.go). The backend checks the
 * shape and that the data is current.
 */
export function openMeteoFloodUrl(at: Coordinates): string {
  const query = new URLSearchParams({
    ...neighbourhood(at, 0.05),
    daily: 'river_discharge',
    past_days: '92',
    forecast_days: '7',
  });
  return `${OPEN_METEO_FLOOD_URL}?${query}`;
}

/**
 * Open-Meteo data is licensed CC BY 4.0, so every place that shows a risk
 * assessment credits it.
 */
export const WEATHER_ATTRIBUTION = {
  label: 'Weather data by Open-Meteo.com',
  url: 'https://open-meteo.com/',
  licence: 'CC BY 4.0',
  licenceUrl: 'https://creativecommons.org/licenses/by/4.0/',
} as const;
