import AsyncStorage from '@react-native-async-storage/async-storage';
import { isCommunityReport, isRiskAssessment } from '@aurarisk/shared';
import { RiskData, CommunityReport, CachedData } from '../types/api';

const CACHE_KEYS = {
  LAST_RISK: '@aurarisk/last_risk',
  LAST_REPORTS: '@aurarisk/last_reports',
};

const MAX_STALE_MS = 24 * 60 * 60 * 1000; // 24 Hours

/**
 * Offline cache limits (docs/MOBILE_PHASE3.md, "Offline storage limits"). Risk and nearby-report
 * entries together are kept to at most MAX_CACHE_ENTRIES entries and MAX_CACHE_BYTES of JSON
 * (counted in UTF-16 code units, about one byte each for this data). The oldest saved entries are
 * evicted first. The report queue and photos are stored separately and are never evicted.
 */
export const MAX_CACHE_ENTRIES = 40;
export const MAX_CACHE_BYTES = 2 * 1024 * 1024;
const INDEX_KEY = '@aurarisk/cache_index';
const CACHE_PREFIXES = Object.values(CACHE_KEYS).map((key) => `${key}_`);

interface IndexEntry {
  key: string;
  savedAt: number;
  bytes: number;
}

const isCacheKey = (key: string) => CACHE_PREFIXES.some((prefix) => key.startsWith(prefix));

const readIndex = async (): Promise<IndexEntry[]> => {
  try {
    const parsed = JSON.parse((await AsyncStorage.getItem(INDEX_KEY)) ?? '[]');
    return Array.isArray(parsed) ? parsed.filter((e) => e && typeof e.key === 'string') : [];
  } catch {
    return [];
  }
};

/** The index, reconciled with what is actually stored (entries written before eviction existed have no index row). */
const loadIndex = async (): Promise<IndexEntry[]> => {
  const keys = (await AsyncStorage.getAllKeys()).filter(isCacheKey);
  const present = new Set(keys);
  const index = (await readIndex()).filter((entry) => present.has(entry.key));
  const indexed = new Set(index.map((entry) => entry.key));
  const missing = keys.filter((key) => !indexed.has(key));
  if (missing.length > 0) {
    for (const [key, raw] of await AsyncStorage.multiGet(missing)) {
      let savedAt = 0;
      try {
        savedAt = Number(JSON.parse(raw ?? '{}').timestamp) || 0;
      } catch {
        // Unreadable entry: treat it as the oldest so it goes first.
      }
      index.push({ key, savedAt, bytes: raw?.length ?? 0 });
    }
  }
  return index;
};

/** Oldest-first keys to evict so the cache fits its limits, never including `keep`. */
export const selectEvictions = (index: IndexEntry[], keep: string): string[] => {
  const sorted = [...index].sort((a, b) => a.savedAt - b.savedAt);
  let count = sorted.length;
  let bytes = sorted.reduce((sum, entry) => sum + entry.bytes, 0);
  const evict: string[] = [];
  for (const entry of sorted) {
    if (count <= MAX_CACHE_ENTRIES && bytes <= MAX_CACHE_BYTES) break;
    if (entry.key === keep) continue;
    evict.push(entry.key);
    count -= 1;
    bytes -= entry.bytes;
  }
  return evict;
};

// Writes update the shared index, so run them one at a time.
let writeChain: Promise<unknown> = Promise.resolve();

const saveEntry = (key: string, payload: unknown): Promise<void> => {
  const run = writeChain.then(async () => {
    const json = JSON.stringify(payload);
    await AsyncStorage.setItem(key, json);
    const index = (await loadIndex()).filter((entry) => entry.key !== key);
    index.push({ key, savedAt: Date.now(), bytes: json.length });
    const evict = selectEvictions(index, key);
    if (evict.length > 0) await AsyncStorage.multiRemove(evict);
    const gone = new Set(evict);
    await AsyncStorage.setItem(INDEX_KEY, JSON.stringify(index.filter((entry) => !gone.has(entry.key))));
  });
  writeChain = run.catch(() => undefined);
  return run;
};

export const saveRiskToCache = async (lat: number, lon: number, data: RiskData): Promise<void> => {
  try {
    const payload: CachedData<RiskData> = {
      timestamp: Date.now(),
      data: { ...data, cachedAt: Date.now() },
    };
    await saveEntry(`${CACHE_KEYS.LAST_RISK}_${lat.toFixed(2)}_${lon.toFixed(2)}`, payload);
  } catch (err) {
    console.error('Failed to cache risk data:', err);
  }
};

export const getCachedRisk = async (lat: number, lon: number): Promise<{ data: RiskData; isStale: boolean } | null> => {
  try {
    const raw = await AsyncStorage.getItem(`${CACHE_KEYS.LAST_RISK}_${lat.toFixed(2)}_${lon.toFixed(2)}`);
    if (!raw) return null;

    const parsed: CachedData<RiskData> = JSON.parse(raw);
    // Entries written by older app versions used a different shape; treat them as a cache miss.
    if (!isRiskAssessment(parsed.data)) return null;
    const isStale = Date.now() - parsed.timestamp > MAX_STALE_MS;

    return { data: parsed.data, isStale };
  } catch (err) {
    console.error('Failed to retrieve cached risk:', err);
    return null;
  }
};

export const saveReportsToCache = async (lat: number, lon: number, reports: CommunityReport[]): Promise<void> => {
  try {
    const payload: CachedData<CommunityReport[]> = {
      timestamp: Date.now(),
      data: reports,
    };
    await saveEntry(`${CACHE_KEYS.LAST_REPORTS}_${lat.toFixed(2)}_${lon.toFixed(2)}`, payload);
  } catch (err) {
    console.error('Failed to cache reports:', err);
  }
};

export const getCachedReports = async (lat: number, lon: number): Promise<CommunityReport[]> => {
  try {
    const raw = await AsyncStorage.getItem(`${CACHE_KEYS.LAST_REPORTS}_${lat.toFixed(2)}_${lon.toFixed(2)}`);
    if (!raw) return [];
    const parsed: CachedData<CommunityReport[]> = JSON.parse(raw);
    return Array.isArray(parsed.data) ? parsed.data.filter(isCommunityReport) : [];
  } catch (err) {
    console.error('Failed to retrieve cached reports:', err);
    return [];
  }
};
