import { markRequestSlow } from './serverStatus';

/**
 * fetch() for foreground requests against an API that may be asleep.
 *
 * The free demo API sleeps after ~15 minutes idle and takes up to a minute to wake. While it
 * wakes, requests either hang or get a 502/503/504 from the proxy. This keeps trying within an
 * overall budget instead of failing after one short timeout:
 * - 502/503/504 are retried with backoff for every method (the app never handled the request).
 * - Timeouts and network errors are retried only for GET/HEAD. A POST that timed out may still
 *   have been processed, so it gets one attempt that may use the whole budget.
 *
 * Background tasks keep their own deadline (see backgroundRefresh.ts) and stop waiting earlier.
 */

export const FOREGROUND_BUDGET_MS = 60_000;
const ATTEMPT_TIMEOUT_MS = 20_000;
const SLOW_AFTER_MS = 3_000;
const BASE_DELAY_MS = 1_000;
const MAX_DELAY_MS = 8_000;
const RETRYABLE_STATUS = new Set([502, 503, 504]);

export interface RetryOptions {
  budgetMs?: number;
  attemptTimeoutMs?: number;
  slowAfterMs?: number;
  baseDelayMs?: number;
  maxDelayMs?: number;
  fetchImpl?: typeof fetch;
}

export class RequestTimeoutError extends Error {
  constructor(url: string, budgetMs: number) {
    super(`Request to ${url} did not complete within ${Math.round(budgetMs / 1000)}s`);
    this.name = 'RequestTimeoutError';
  }
}

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

const isIdempotent = (method?: string) => {
  const m = (method ?? 'GET').toUpperCase();
  return m === 'GET' || m === 'HEAD';
};

export const fetchWithRetry = async (url: string, init: RequestInit = {}, options: RetryOptions = {}): Promise<Response> => {
  const {
    budgetMs = FOREGROUND_BUDGET_MS,
    attemptTimeoutMs = ATTEMPT_TIMEOUT_MS,
    slowAfterMs = SLOW_AFTER_MS,
    baseDelayMs = BASE_DELAY_MS,
    maxDelayMs = MAX_DELAY_MS,
    fetchImpl = fetch,
  } = options;

  const deadline = Date.now() + budgetMs;
  const retryOnTimeout = isIdempotent(init.method);
  let endSlow: (() => void) | undefined;
  const markSlow = () => {
    endSlow ??= markRequestSlow();
  };
  const slowTimer = setTimeout(markSlow, slowAfterMs);
  let attempt = 0;

  try {
    for (;;) {
      const remaining = deadline - Date.now();
      if (remaining <= 0) throw new RequestTimeoutError(url, budgetMs);

      // A non-idempotent request gets one attempt, so let it wait for the whole budget.
      const timeoutMs = retryOnTimeout ? Math.min(attemptTimeoutMs, remaining) : remaining;
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), timeoutMs);
      let response: Response | undefined;
      let failure: unknown;
      try {
        response = await fetchImpl(url, { ...init, signal: controller.signal });
      } catch (err) {
        failure = err;
      } finally {
        clearTimeout(timer);
      }

      if (response && !RETRYABLE_STATUS.has(response.status)) return response;
      if (!response && !retryOnTimeout) {
        throw controller.signal.aborted ? new RequestTimeoutError(url, budgetMs) : failure;
      }

      // Retryable: the server is (probably) still waking.
      markSlow();
      const delay = Math.min(baseDelayMs * 2 ** attempt, maxDelayMs);
      attempt += 1;
      if (Date.now() + delay >= deadline) {
        if (response) return response; // let the caller see the 50x
        throw controller.signal.aborted ? new RequestTimeoutError(url, budgetMs) : failure;
      }
      await sleep(delay);
    }
  } finally {
    clearTimeout(slowTimer);
    endSlow?.();
  }
};
