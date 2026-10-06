import { ApiError, createApiClient, type FieldErrors, validateNewReport, ValidationError } from '@aurarisk/shared';
import NetInfo from '@react-native-community/netinfo';
import { authFetch } from '../auth/authClient';
import { API_BASE_URL, fetchWithTimeout } from '../config';
import { uploadReportPhotos } from '../photos/upload';
import { RiskData, CommunityReport } from '../types/api';
import { saveRiskToCache, getCachedRisk, saveReportsToCache, getCachedReports } from '../storage/cache';
import { enqueueReport, NewReport, QueuedReport } from '../storage/reportQueue';

/** Unauthenticated endpoints (risk, nearby reports). */
export const publicApi = createApiClient({ baseUrl: API_BASE_URL, fetch: fetchWithTimeout });

// authFetch prefixes API_BASE_URL itself and handles token refresh, so this client sends bare paths.
const deviceApi = createApiClient({ baseUrl: '', fetch: authFetch });

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
    const data: RiskData = await publicApi.getRisk({ lat, lon });
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
    const reports: CommunityReport[] = await publicApi.getReports({ lat, lon }, radius);
    saveReportsToCache(lat, lon, reports);

    return { reports, isOffline: false };
  } catch {
    const cachedReports = await getCachedReports(lat, lon);
    return { reports: cachedReports, isOffline: true };
  }
};

export class ReportRejectedError extends Error {}

/** The report failed local validation; `errors` is keyed by field (name, lat, lon, category, note). */
export class ReportValidationError extends ReportRejectedError {
  constructor(readonly errors: FieldErrors) {
    super(Object.values(errors)[0] ?? 'Invalid report');
  }
}

/** Creates the report as the signed-in device account and returns its server id. */
export const createReport = async (report: NewReport): Promise<string> => {
  try {
    const created = await deviceApi.createReport({
      location: report.location,
      category: report.category,
      note: report.note,
    });
    return created.id;
  } catch (err) {
    // Invalid input, or a 4xx (other than auth/rate limiting), means retrying won't help.
    if (err instanceof ValidationError) throw new ReportRejectedError(err.message);
    if (err instanceof ApiError && err.status >= 400 && err.status < 500 && err.status !== 401 && err.status !== 429) {
      throw new ReportRejectedError(err.message);
    }
    throw err;
  }
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
export const submitReport = async (input: NewReport, photos: string[] = []): Promise<SubmitResult> => {
  // Validate before queueing so an invalid report is never stored offline only to be dropped later.
  const checked = validateNewReport(input);
  if (!checked.ok) throw new ReportValidationError(checked.errors);
  const report = checked.value;

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
