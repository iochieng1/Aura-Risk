import AsyncStorage from '@react-native-async-storage/async-storage';
import {
  getCachedReports,
  getCachedRisk,
  MAX_CACHE_BYTES,
  MAX_CACHE_ENTRIES,
  saveReportsToCache,
  saveRiskToCache,
  selectEvictions,
} from '../cache';
import { enqueueReport, getReportQueue } from '../reportQueue';
import { RiskData } from '../../types/api';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

const risk = (lat: number): RiskData =>
  ({
    location: { name: `Place ${lat}`, lat, lon: 36.8 },
    score: 40,
    level: 'Advisory',
    summary: 'Some rain',
    tips: ['Stay alert'],
  }) as RiskData;

const cacheKeys = async () => (await AsyncStorage.getAllKeys()).filter((k) => k.startsWith('@aurarisk/last_'));

let now = 1_000_000;

describe('offline cache eviction', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    now = 1_000_000;
    jest.spyOn(Date, 'now').mockImplementation(() => (now += 1000));
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('keeps at most MAX_CACHE_ENTRIES entries and evicts the oldest first', async () => {
    for (let i = 0; i < MAX_CACHE_ENTRIES + 5; i++) await saveRiskToCache(i, 36.8, risk(i));

    expect(await cacheKeys()).toHaveLength(MAX_CACHE_ENTRIES);
    for (let i = 0; i < 5; i++) expect(await getCachedRisk(i, 36.8)).toBeNull();
    expect((await getCachedRisk(5, 36.8))?.data.location.name).toBe('Place 5');
    expect((await getCachedRisk(MAX_CACHE_ENTRIES + 4, 36.8))?.data.location.name).toBe(`Place ${MAX_CACHE_ENTRIES + 4}`);
  });

  it('refreshing an entry makes it the newest', async () => {
    for (let i = 0; i < MAX_CACHE_ENTRIES; i++) await saveRiskToCache(i, 36.8, risk(i));
    await saveRiskToCache(0, 36.8, risk(0));
    await saveRiskToCache(100, 36.8, risk(100));

    expect(await getCachedRisk(0, 36.8)).not.toBeNull();
    expect(await getCachedRisk(1, 36.8)).toBeNull();
  });

  it('keeps the total size under MAX_CACHE_BYTES', async () => {
    const bigNote = 'x'.repeat(200_000);
    const reports = [{ id: 'r', location: { name: 'p', lat: 0, lon: 0 }, category: 'flooding', note: bigNote, created_at: '' }];
    for (let i = 0; i < 15; i++) await saveReportsToCache(i, 36.8, reports as never);

    const stored = await AsyncStorage.multiGet(await cacheKeys());
    const bytes = stored.reduce((sum, [, v]) => sum + (v?.length ?? 0), 0);
    expect(bytes).toBeLessThanOrEqual(MAX_CACHE_BYTES);
    expect(stored.length).toBeLessThan(15);
    expect(await getCachedReports(0, 36.8)).toEqual([]);
  });

  it('counts entries written before eviction existed, oldest first', async () => {
    // Legacy entry with no index row.
    await AsyncStorage.setItem('@aurarisk/last_risk_-1.00_36.80', JSON.stringify({ timestamp: 1, data: risk(-1) }));
    for (let i = 0; i < MAX_CACHE_ENTRIES; i++) await saveRiskToCache(i, 36.8, risk(i));

    expect(await cacheKeys()).toHaveLength(MAX_CACHE_ENTRIES);
    expect(await getCachedRisk(-1, 36.8)).toBeNull();
    expect(await getCachedRisk(0, 36.8)).not.toBeNull();
  });

  it('never touches the report queue', async () => {
    await enqueueReport({ location: { name: 'Bridge', lat: 0, lon: 0 }, category: 'flooding', note: 'Water' });
    for (let i = 0; i < MAX_CACHE_ENTRIES + 5; i++) await saveRiskToCache(i, 36.8, risk(i));

    expect(await getReportQueue()).toHaveLength(1);
  });

  it('never evicts the entry just written, even if it alone is over the limit', () => {
    const index = [
      { key: 'a', savedAt: 1, bytes: 10 },
      { key: 'b', savedAt: 2, bytes: MAX_CACHE_BYTES + 1 },
    ];
    expect(selectEvictions(index, 'b')).toEqual(['a']);
  });
});
