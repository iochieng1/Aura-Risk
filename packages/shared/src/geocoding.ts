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

/**
 * Nominatim's usage policy (https://operations.osmfoundation.org/policies/nominatim/) allows at
 * most one request per second per client, requires caching repeated queries, and forbids
 * search-as-you-type. Callers must search on an explicit submit; this module enforces the rate
 * and the cache.
 */
export const PLACE_SEARCH_MIN_INTERVAL_MS = 1000;
const CACHE_SIZE = 50;

interface PlaceSearchDeps {
  now: () => number;
  sleep: (ms: number) => Promise<void>;
}

/** Builds a place search with its own rate limit and cache. Exposed for tests. */
export function createPlaceSearch(deps: PlaceSearchDeps = defaultDeps) {
  let nextSlot = 0;
  const cache = new Map<string, Location[]>();

  // Reserves the next free slot synchronously so concurrent callers queue up.
  const waitForSlot = async () => {
    const now = deps.now();
    const at = Math.max(now, nextSlot);
    nextSlot = at + PLACE_SEARCH_MIN_INTERVAL_MS;
    if (at > now) await deps.sleep(at - now);
  };

  /** Free-text place search via OpenStreetMap Nominatim. Returns [] on short queries or errors. */
  return async function searchPlaces(query: string, options: SearchPlacesOptions = {}): Promise<Location[]> {
    const q = query.trim();
    if (q.length < MIN_PLACE_QUERY_LENGTH) return [];

    const limit = options.limit ?? 5;
    const key = `${limit}:${q.toLowerCase()}`;
    const cached = cache.get(key);
    if (cached) return cached;

    const doFetch: FetchLike = options.fetch ?? ((url, init) => fetch(url, init));
    const url = `https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(q)}&limit=${limit}&addressdetails=0`;
    const headers: Record<string, string> = { 'Accept-Language': 'en' };
    if (options.userAgent) headers['User-Agent'] = options.userAgent;

    try {
      await waitForSlot();
      const res = await doFetch(url, { headers });
      if (!res.ok) return [];
      const data: NominatimResult[] = await res.json();
      const places = data
        .map((r) => ({ name: r.display_name, lat: parseFloat(r.lat), lon: parseFloat(r.lon) }))
        .filter((l) => Number.isFinite(l.lat) && Number.isFinite(l.lon));
      if (cache.size >= CACHE_SIZE) cache.delete(cache.keys().next().value!);
      cache.set(key, places);
      return places;
    } catch {
      return [];
    }
  };
}

const defaultDeps: PlaceSearchDeps = {
  now: () => Date.now(),
  sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
};

export const searchPlaces = createPlaceSearch();
