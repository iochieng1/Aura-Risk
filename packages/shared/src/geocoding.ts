import type { FetchLike } from './client';
import type { Location } from './types';

interface NominatimResult {
  display_name: string;
  lat: string;
  lon: string;
}

export interface SearchPlacesOptions {
  fetch?: FetchLike;
  /**
   * Nominatim's usage policy requires an identifying User-Agent. Browsers set their own and
   * ignore this; native apps must pass one.
   */
  userAgent?: string;
  limit?: number;
}

export const MIN_PLACE_QUERY_LENGTH = 3;

/** Free-text place search via OpenStreetMap Nominatim. Returns [] on short queries or errors. */
export async function searchPlaces(query: string, options: SearchPlacesOptions = {}): Promise<Location[]> {
  const q = query.trim();
  if (q.length < MIN_PLACE_QUERY_LENGTH) return [];

  const doFetch: FetchLike = options.fetch ?? ((url, init) => fetch(url, init));
  const url = `https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(q)}&limit=${
    options.limit ?? 5
  }&addressdetails=0`;
  const headers: Record<string, string> = { 'Accept-Language': 'en' };
  if (options.userAgent) headers['User-Agent'] = options.userAgent;

  try {
    const res = await doFetch(url, { headers });
    if (!res.ok) return [];
    const data: NominatimResult[] = await res.json();
    return data
      .map((r) => ({ name: r.display_name, lat: parseFloat(r.lat), lon: parseFloat(r.lon) }))
      .filter((l) => Number.isFinite(l.lat) && Number.isFinite(l.lon));
  } catch {
    return [];
  }
}
