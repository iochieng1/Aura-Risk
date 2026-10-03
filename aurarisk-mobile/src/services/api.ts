import NetInfo from '@react-native-community/netinfo';
import { authFetch } from '../auth/authClient';
import { API_BASE_URL, fetchWithTimeout } from '../config';
import { uploadReportPhotos } from '../photos/upload';
import { RiskData, CommunityReport } from '../types/api';
import { saveRiskToCache, getCachedRisk, saveReportsToCache, getCachedReports } from '../storage/cache';
import { enqueueReport, NewReport, QueuedReport } from '../storage/reportQueue';

export interface RiskFetchResult {
  data: RiskData | null;
  isOffline: boolean;
  isStale: boolean;
  error?: string;
}

export const isOnline = async (): Promise<boolean> => {
  const netState = await NetInfo.fetch();
  return Boolean(netState.isConnected && netState.isInternetReachable !== false);
};

export const fetchRiskAssessment = async (lat: number, lon: number): Promise<RiskFetchResult> => {
  if (!(await isOnline())) {
    const cached = await getCachedRisk(lat, lon);
    return cached
      ? { data: cached.data, isOffline: true, isStale: cached.isStale }
      : { data: null, isOffline: true, isStale: false, error: 'No offline cache available' };
  }

  try {
    const response = await fetchWithTimeout(`${API_BASE_URL}/api/risk?lat=${lat}&lon=${lon}`);
    if (!response.ok) throw new Error(`HTTP error ${response.status}`);

    const data: RiskData = await response.json();
    await saveRiskToCache(lat, lon, data);

    return { data, isOffline: false, isStale: false };
  } catch {
    const cached = await getCachedRisk(lat, lon);
    return cached
      ? { data: cached.data, isOffline: true, isStale: cached.isStale }
      : { data: null, isOffline: true, isStale: false, error: 'Failed to fetch' };
  }
};

export const fetchNearbyReports = async (lat: number, lon: number, radius = 10): Promise<{ reports: CommunityReport[]; isOffline: boolean }> => {
  if (!(await isOnline())) {
    const cachedReports = await getCachedReports(lat, lon);
    return { reports: cachedReports, isOffline: true };
  }

  try {
    const response = await fetchWithTimeout(`${API_BASE_URL}/api/reports?lat=${lat}&lon=${lon}&radius=${radius}`);
    if (!response.ok) throw new Error(`HTTP error ${response.status}`);

    const reports: CommunityReport[] = await response.json();
    saveReportsToCache(lat, lon, reports);

    return { reports, isOffline: false };
  } catch {
    const cachedReports = await getCachedReports(lat, lon);
    return { reports: cachedReports, isOffline: true };
  }
};

export class ReportRejectedError extends Error {}

/** Creates the report as the signed-in device account and returns its server id. */
export const createReport = async (report: NewReport): Promise<string> => {
  const response = await authFetch('/api/reports', {
    method: 'POST',
    body: JSON.stringify({
      location: { name: report.location.name, lat: report.location.lat, lon: report.location.lon },
      category: report.category,
      note: report.note,
    }),
  });

  if (!response.ok) {
    const message = `Server status ${response.status}`;
    // 4xx (other than auth/rate limiting) means the report itself is invalid; retrying won't help.
    if (response.status >= 400 && response.status < 500 && response.status !== 401 && response.status !== 429) {
      throw new ReportRejectedError(message);
    }
    throw new Error(message);
  }
  const created: CommunityReport = await response.json();
  if (!created.id) throw new Error('Server did not return a report id');
  return created.id;
};

export interface SubmitResult {
  success: boolean;
  queued: boolean;
  reportId?: string;
  item?: QueuedReport;
}

/**
 * Submits a report with optional photos (local URIs from photoStore). Anything that can't be sent
 * right now is queued. If the report was created but a photo failed, the queued item carries the
 * server id so a retry only uploads the remaining photos.
 */
export const submitReport = async (report: NewReport, photos: string[] = []): Promise<SubmitResult> => {
  if (!(await isOnline())) {
    const item = await enqueueReport(report, photos);
    return { success: true, queued: true, item };
  }

  let reportId: string;
  try {
    reportId = await createReport(report);
  } catch (err) {
    if (err instanceof ReportRejectedError) throw err;
    const item = await enqueueReport(report, photos);
    return { success: true, queued: true, item };
  }

  const remaining = [...photos];
  try {
    await uploadReportPhotos(reportId, photos, (uri) => {
      remaining.splice(remaining.indexOf(uri), 1);
    });
  } catch {
    const item = await enqueueReport(report, remaining, reportId);
    return { success: true, queued: true, reportId, item };
  }

  return { success: true, queued: false, reportId };
};
