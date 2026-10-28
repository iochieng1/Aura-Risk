import { DEFAULT_REPORT_RADIUS_KM } from './constants';
import type { Lang } from './i18n';
import type { CommunityReport, Coordinates, CountyStatsResponse, Location, NewReportInput, RiskAssessment } from './types';
import { type FieldErrors, isCommunityReport, isRiskAssessment, validateNewReport } from './validation';
import { openMeteoForecastUrl } from './weather';

export type FetchLike = (url: string, init?: RequestInit) => Promise<Response>;

export interface ApiClientOptions {
  /** Origin of the backend, e.g. "http://10.0.2.2:8080". Use "" for same-origin requests. */
  baseUrl: string;
  /** Override to add timeouts, auth headers, or test doubles. Defaults to global fetch. */
  fetch?: FetchLike;
  /**
   * When GET /api/risk answers 503 because the server can't get weather data
   * (e.g. its shared IP is over Open-Meteo's free daily limit), fetch the
   * weather from Open-Meteo on this device and have the server score it with
   * POST /api/risk. Default true.
   */
  weatherFallback?: boolean;
  /**
   * The user's language, read on each request. GET and POST /api/risk return
   * the summary and tips in it. Unset leaves it to the server (English).
   */
  lang?: () => Lang | undefined;
}

/** The server answered with a non-2xx status. `message` is the backend's `error` field when present. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    /** Machine-readable code from the backend, e.g. "not_found", "too_many_requests". */
    readonly code?: string,
    /** Quote this when reporting a problem; it finds the request in backend logs. */
    readonly requestId?: string,
    /** Seconds the backend asked to wait (Retry-After) on 429 and 503. */
    readonly retryAfter?: number
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/** The request was rejected locally before anything was sent. */
export class ValidationError extends Error {
  constructor(readonly errors: FieldErrors) {
    super(Object.values(errors)[0] ?? 'Invalid input');
    this.name = 'ValidationError';
  }
}

/** The server answered 2xx but the body did not match the expected shape. */
export class MalformedResponseError extends Error {
  constructor(what: string) {
    super(`Malformed ${what} response`);
    this.name = 'MalformedResponseError';
  }
}

export interface ApiClient {
  /**
   * GET /api/risk. When `at` has a name it replaces the server's placeholder
   * ("Selected Location"), since the backend does not geocode. Falls back
   * to device-fetched weather on a 503 (see `weatherFallback`).
   */
  getRisk(at: Coordinates | Location): Promise<RiskAssessment>;
  /** GET /api/reports. Newest first, at most 50. */
  getReports(at: Coordinates, radiusKm?: number): Promise<CommunityReport[]>;
  /** POST /api/reports. Validates first and throws ValidationError without sending. */
  createReport(input: NewReportInput): Promise<CommunityReport>;
  /** GET /api/stats/counties. Dates are YYYY-MM-DD; the server defaults to the last 90 days. */
  getCountyStats(period?: StatsPeriod): Promise<CountyStatsResponse>;
  /** Download link for the open data export (CC BY 4.0). */
  openDataUrl(format: 'csv' | 'json', period?: StatsPeriod & { county?: string }): string;
}

/** Dates are YYYY-MM-DD, inclusive. */
export type StatsPeriod = {
  from?: string;
  to?: string;
};

const readError = async (response: Response): Promise<ApiError> => {
  let message = `Request failed (${response.status})`;
  let code: string | undefined;
  let requestId = response.headers?.get('X-Request-ID') ?? undefined;
  try {
    const body = await response.json();
    if (body && typeof body.error === 'string') message = body.error;
    if (body && typeof body.code === 'string') code = body.code;
    if (body && typeof body.request_id === 'string') requestId = body.request_id;
  } catch {
    // Non-JSON error body; keep the generic message.
  }
  const retryAfter = Number(response.headers?.get('Retry-After'));
  return new ApiError(response.status, message, code, requestId, retryAfter > 0 ? retryAfter : undefined);
};

export function createApiClient({ baseUrl, fetch: fetchImpl, weatherFallback = true, lang }: ApiClientOptions): ApiClient {
  const doFetch: FetchLike = fetchImpl ?? ((url, init) => fetch(url, init));
  const root = baseUrl.replace(/\/+$/, '');

  const request = async (path: string, init?: RequestInit): Promise<unknown> => {
    const response = await doFetch(`${root}${path}`, init);
    if (!response.ok) throw await readError(response);
    return response.json();
  };

  const coordQuery = ({ lat, lon }: Coordinates) => `lat=${encodeURIComponent(lat)}&lon=${encodeURIComponent(lon)}`;
  const langQuery = (separator: '?' | '&') => {
    const value = lang?.();
    return value ? `${separator}lang=${value}` : '';
  };

  // Fetches weather on this device and asks the server to score it. If that
  // fails too, the server's original 503 is what the caller sees.
  const riskFromDeviceWeather = async ({ lat, lon }: Coordinates, serverError: ApiError): Promise<unknown> => {
    let weather: unknown;
    try {
      const response = await doFetch(openMeteoForecastUrl({ lat, lon }));
      if (!response.ok) throw serverError;
      weather = await response.json();
    } catch {
      throw serverError;
    }
    return request(`/api/risk${langQuery('?')}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ lat, lon, weather }),
    });
  };

  return {
    async getRisk(at) {
      let body: unknown;
      try {
        body = await request(`/api/risk?${coordQuery(at)}${langQuery('&')}`);
      } catch (err) {
        if (!weatherFallback || !(err instanceof ApiError) || err.status !== 503) throw err;
        body = await riskFromDeviceWeather(at, err);
      }
      if (!isRiskAssessment(body)) throw new MalformedResponseError('risk');
      const name = 'name' in at && at.name ? at.name : body.location.name;
      return { ...body, location: { ...body.location, name } };
    },

    async getReports(at, radiusKm = DEFAULT_REPORT_RADIUS_KM) {
      const body = await request(`/api/reports?${coordQuery(at)}&radius=${encodeURIComponent(radiusKm)}`);
      if (!Array.isArray(body)) throw new MalformedResponseError('reports');
      return body.filter(isCommunityReport);
    },

    async createReport(input) {
      const checked = validateNewReport(input);
      if (!checked.ok) throw new ValidationError(checked.errors);
      const body = await request('/api/reports', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(checked.value),
      });
      if (!isCommunityReport(body)) throw new MalformedResponseError('report');
      return body;
    },

    async getCountyStats(period = {}) {
      const body = await request(`/api/stats/counties${periodQuery(period)}`);
      if (!body || typeof body !== 'object' || !Array.isArray((body as CountyStatsResponse).counties)) {
        throw new MalformedResponseError('county stats');
      }
      return body as CountyStatsResponse;
    },

    openDataUrl(format, period = {}) {
      return `${root}/api/open-data/reports.${format}${periodQuery(period)}`;
    },
  };
}

const periodQuery = (params: Record<string, string | undefined>): string => {
  const query = Object.entries(params)
    .filter((entry): entry is [string, string] => !!entry[1])
    .map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
    .join('&');
  return query ? `?${query}` : '';
};
