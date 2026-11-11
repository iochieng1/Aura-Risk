import { fetchWithRetry, RequestTimeoutError } from '../fetchWithRetry';
import { isServerWaking } from '../serverStatus';

const ok = (status = 200) => new Response('{}', { status });

/** A fetch that never answers until aborted, like a request held by a sleeping server. */
const hangingFetch = (_url: RequestInfo | URL, init?: RequestInit) =>
  new Promise<Response>((_resolve, reject) => {
    init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
  });

// Short timings keep the tests fast; the production defaults are 60s budget / 20s attempts.
const fast = { budgetMs: 400, attemptTimeoutMs: 100, slowAfterMs: 30, baseDelayMs: 10, maxDelayMs: 40 };

describe('fetchWithRetry', () => {
  it('returns a slow first response instead of timing out early', async () => {
    const fetchImpl = jest.fn(
      () => new Promise<Response>((resolve) => setTimeout(() => resolve(ok()), 60))
    ) as unknown as typeof fetch;

    const response = await fetchWithRetry('https://api.test/api/risk', {}, { ...fast, fetchImpl });

    expect(response.status).toBe(200);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
    expect(isServerWaking()).toBe(false);
  });

  it('retries repeated 503s until the server is up', async () => {
    const fetchImpl = jest
      .fn()
      .mockResolvedValueOnce(ok(503))
      .mockResolvedValueOnce(ok(502))
      .mockResolvedValueOnce(ok(503))
      .mockResolvedValueOnce(ok(200)) as unknown as typeof fetch;

    const response = await fetchWithRetry('https://api.test/api/risk', {}, { ...fast, fetchImpl });

    expect(response.status).toBe(200);
    expect(fetchImpl).toHaveBeenCalledTimes(4);
    expect(isServerWaking()).toBe(false);
  });

  it('retries a GET that times out', async () => {
    const fetchImpl = jest
      .fn()
      .mockImplementationOnce(hangingFetch)
      .mockResolvedValueOnce(ok(200)) as unknown as typeof fetch;

    const response = await fetchWithRetry('https://api.test/health', {}, { ...fast, fetchImpl });

    expect(response.status).toBe(200);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it('gives up with the last 50x once the budget is spent', async () => {
    const fetchImpl = jest.fn().mockResolvedValue(ok(503)) as unknown as typeof fetch;

    const response = await fetchWithRetry('https://api.test/api/risk', {}, { ...fast, budgetMs: 150, fetchImpl });

    expect(response.status).toBe(503);
    expect((fetchImpl as jest.Mock).mock.calls.length).toBeGreaterThan(1);
    expect(isServerWaking()).toBe(false);
  });

  it('gives up with a timeout error when the server never answers', async () => {
    const fetchImpl = jest.fn(hangingFetch) as unknown as typeof fetch;

    await expect(fetchWithRetry('https://api.test/api/risk', {}, { ...fast, fetchImpl })).rejects.toBeInstanceOf(
      RequestTimeoutError
    );
    expect(isServerWaking()).toBe(false);
  });

  it('does not resend a POST that timed out, but waits the full budget for it', async () => {
    const fetchImpl = jest.fn(hangingFetch) as unknown as typeof fetch;
    const started = Date.now();

    await expect(
      fetchWithRetry('https://api.test/api/auth/device', { method: 'POST' }, { ...fast, budgetMs: 200, fetchImpl })
    ).rejects.toBeInstanceOf(RequestTimeoutError);

    expect(fetchImpl).toHaveBeenCalledTimes(1);
    expect(Date.now() - started).toBeGreaterThanOrEqual(190);
  });

  it('retries a POST on 503, because the server never handled it', async () => {
    const fetchImpl = jest.fn().mockResolvedValueOnce(ok(503)).mockResolvedValueOnce(ok(201)) as unknown as typeof fetch;

    const response = await fetchWithRetry('https://api.test/api/auth/device', { method: 'POST' }, { ...fast, fetchImpl });

    expect(response.status).toBe(201);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it('reports the server as waking while a request is slow', async () => {
    let release: (r: Response) => void = () => {};
    const fetchImpl = jest.fn(() => new Promise<Response>((resolve) => (release = resolve))) as unknown as typeof fetch;

    const pending = fetchWithRetry('https://api.test/api/risk', {}, { ...fast, fetchImpl });
    await new Promise((r) => setTimeout(r, 50));
    expect(isServerWaking()).toBe(true);

    release(ok());
    await pending;
    expect(isServerWaking()).toBe(false);
  });
});
