import { DEFAULT_REPORT_RADIUS_KM } from './constants';
import type { CommunityReport, Coordinates, Location, NewReportInput, RiskAssessment } from './types';
import { type FieldErrors, isCommunityReport, isRiskAssessment, validateNewReport } from './validation';

export type FetchLike = (url: string, init?: RequestInit) => Promise<Response>;

export interface ApiClientOptions {
  /** Origin of the backend, e.g. "http://10.0.2.2:8080". Use "" for same-origin requests. */
  baseUrl: string;
  /** Override to add timeouts, auth headers, or test doubles. Defaults to global fetch. */
  fetch?: FetchLike;
}

/** The server answered with a non-2xx status. `message` is the backend's `error` field when present. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string
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
   * ("Selected Location"), since the backend does not geocode.
   */
  getRisk(at: Coordinates | Location): Promise<RiskAssessment>;
  /** GET /api/reports. Newest first, at most 50. */
  getReports(at: Coordinates, radiusKm?: number): Promise<CommunityReport[]>;
  /** POST /api/reports. Validates first and throws ValidationError without sending. */
  createReport(input: NewReportInput): Promise<CommunityReport>;
}

const readError = async (response: Response): Promise<ApiError> => {
  let message = `Request failed (${response.status})`;
  try {
    const body = await response.json();
    if (body && typeof body.error === 'string') message = body.error;
  } catch {
    // Non-JSON error body; keep the generic message.
  }
  return new ApiError(response.status, message);
};

export function createApiClient({ baseUrl, fetch: fetchImpl }: ApiClientOptions): ApiClient {
  const doFetch: FetchLike = fetchImpl ?? ((url, init) => fetch(url, init));
  const root = baseUrl.replace(/\/+$/, '');

  const request = async (path: string, init?: RequestInit): Promise<unknown> => {
    const response = await doFetch(`${root}${path}`, init);
    if (!response.ok) throw await readError(response);
    return response.json();
  };

  const coordQuery = ({ lat, lon }: Coordinates) => `lat=${encodeURIComponent(lat)}&lon=${encodeURIComponent(lon)}`;

  return {
    async getRisk(at) {
      const body = await request(`/api/risk?${coordQuery(at)}`);
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
  };
}
