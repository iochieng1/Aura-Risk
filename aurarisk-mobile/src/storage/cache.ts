import AsyncStorage from '@react-native-async-storage/async-storage';
import { RiskData, CommunityReport, CachedData } from '../types/api';

const CACHE_KEYS = {
  LAST_RISK: '@aurarisk/last_risk',
  LAST_REPORTS: '@aurarisk/last_reports',
};

const MAX_STALE_MS = 24 * 60 * 60 * 1000; // 24 Hours

export const saveRiskToCache = async (lat: number, lon: number, data: RiskData): Promise<void> => {
  try {
    const payload: CachedData<RiskData> = {
      timestamp: Date.now(),
      data: { ...data, cachedAt: Date.now() },
    };
    await AsyncStorage.setItem(`${CACHE_KEYS.LAST_RISK}_${lat.toFixed(2)}_${lon.toFixed(2)}`, JSON.stringify(payload));
  } catch (err) {
    console.error('Failed to cache risk data:', err);
  }
};

export const getCachedRisk = async (lat: number, lon: number): Promise<{ data: RiskData; isStale: boolean } | null> => {
  try {
    const raw = await AsyncStorage.getItem(`${CACHE_KEYS.LAST_RISK}_${lat.toFixed(2)}_${lon.toFixed(2)}`);
    if (!raw) return null;

    const parsed: CachedData<RiskData> = JSON.parse(raw);
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
    await AsyncStorage.setItem(`${CACHE_KEYS.LAST_REPORTS}_${lat.toFixed(2)}_${lon.toFixed(2)}`, JSON.stringify(payload));
  } catch (err) {
    console.error('Failed to cache reports:', err);
  }
};

export const getCachedReports = async (lat: number, lon: number): Promise<CommunityReport[]> => {
  try {
    const raw = await AsyncStorage.getItem(`${CACHE_KEYS.LAST_REPORTS}_${lat.toFixed(2)}_${lon.toFixed(2)}`);
    if (!raw) return [];
    const parsed: CachedData<CommunityReport[]> = JSON.parse(raw);
    return parsed.data;
  } catch (err) {
    console.error('Failed to retrieve cached reports:', err);
    return [];
  }
};
