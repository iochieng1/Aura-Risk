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
