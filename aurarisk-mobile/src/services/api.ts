import NetInfo from '@react-native-community/netinfo';
import { RiskData, CommunityReport } from '../types/api';
import { saveRiskToCache, getCachedRisk, saveReportsToCache, getCachedReports } from '../storage/cache';
import { enqueueReport, QueuedReport } from '../storage/reportQueue';

const API_BASE_URL = process.env.EXPO_PUBLIC_API_BASE_URL || 'http://localhost:8080';

export interface RiskFetchResult {
  data: RiskData | null;
  isOffline: boolean;
  isStale: boolean;
  error?: string;
}

export const fetchRiskAssessment = async (lat: number, lon: number): Promise<RiskFetchResult> => {
  const netState = await NetInfo.fetch();

  if (!netState.isConnected || !netState.isInternetReachable) {
    const cached = await getCachedRisk(lat, lon);
    return cached
      ? { data: cached.data, isOffline: true, isStale: cached.isStale }
      : { data: null, isOffline: true, isStale: false, error: 'No offline cache available' };
  }

  try {
    const response = await fetch(`${API_BASE_URL}/api/risk?lat=${lat}&lon=${lon}`);
    if (!response.ok) throw new Error(`HTTP error ${response.status}`);

    const data: RiskData = await response.json();
    saveRiskToCache(lat, lon, data);

    return { data, isOffline: false, isStale: false };
  } catch (err) {
    const cached = await getCachedRisk(lat, lon);
    return cached
      ? { data: cached.data, isOffline: true, isStale: cached.isStale }
      : { data: null, isOffline: true, isStale: false, error: 'Failed to fetch' };
  }
};

export const fetchNearbyReports = async (lat: number, lon: number, radius = 10): Promise<{ reports: CommunityReport[]; isOffline: boolean }> => {
  const netState = await NetInfo.fetch();

  if (!netState.isConnected || !netState.isInternetReachable) {
    const cachedReports = await getCachedReports(lat, lon);
    return { reports: cachedReports, isOffline: true };
  }

  try {
    const response = await fetch(`${API_BASE_URL}/api/reports?lat=${lat}&lon=${lon}&radius=${radius}`);
    if (!response.ok) throw new Error(`HTTP error ${response.status}`);

    const reports: CommunityReport[] = await response.json();
    saveReportsToCache(lat, lon, reports);

    return { reports, isOffline: false };
  } catch {
    const cachedReports = await getCachedReports(lat, lon);
    return { reports: cachedReports, isOffline: true };
  }
};

export const submitReport = async (
  report: Omit<CommunityReport, 'createdAt'>,
  isRetry = false
): Promise<{ success: boolean; queued: boolean; item?: QueuedReport }> => {
  const netState = await NetInfo.fetch();

  if (!netState.isConnected || !netState.isInternetReachable) {
    if (!isRetry) {
      const queuedItem = await enqueueReport(report);
      return { success: true, queued: true, item: queuedItem };
    }
    return { success: false, queued: false };
  }

  try {
    const response = await fetch(`${API_BASE_URL}/api/reports`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(report),
    });

    if (!response.ok) throw new Error(`Server status ${response.status}`);
    return { success: true, queued: false };
  } catch (err) {
    if (!isRetry) {
      const queuedItem = await enqueueReport(report);
      return { success: true, queued: true, item: queuedItem };
    }
    throw err;
  }
};
