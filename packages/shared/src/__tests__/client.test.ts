import { ApiError, createApiClient, MalformedResponseError, ValidationError } from '../client';

const json = (body: unknown, status = 200) =>
  ({ ok: status >= 200 && status < 300, status, json: async () => body }) as Response;

const risk = {
  location: { name: 'Selected Location', lat: -1.29, lon: 36.82 },
  score: 62,
  level: 'Alert',
  summary: 'Heavy rain expected',
  tips: ['Avoid low roads'],
};

describe('createApiClient', () => {
  it('builds risk URLs from the base URL and keeps the caller’s place name', async () => {
    const fetch = jest.fn(async () => json(risk));
    const client = createApiClient({ baseUrl: 'http://api.test/', fetch });

    const result = await client.getRisk({ name: 'Nairobi', lat: -1.29, lon: 36.82 });

    expect(fetch).toHaveBeenCalledWith('http://api.test/api/risk?lat=-1.29&lon=36.82', undefined);
    expect(result.location.name).toBe('Nairobi');
    expect(result.level).toBe('Alert');
  });

  it('surfaces the backend error message and status', async () => {
    const client = createApiClient({ baseUrl: '', fetch: async () => json({ error: 'invalid lat' }, 400) });
    await expect(client.getRisk({ lat: 0, lon: 0 })).rejects.toEqual(new ApiError(400, 'invalid lat'));
  });

  it('keeps the error code, request ID and Retry-After', async () => {
    const response = {
      ...json({ error: 'too many requests, slow down', code: 'too_many_requests', request_id: 'abc123' }, 429),
      headers: new Headers({ 'Retry-After': '12' }),
    } as Response;
    const client = createApiClient({ baseUrl: '', fetch: async () => response });
    const err = await client.getRisk({ lat: 0, lon: 0 }).catch((e: ApiError) => e);
    expect(err).toMatchObject({ status: 429, code: 'too_many_requests', requestId: 'abc123', retryAfter: 12 });
  });

  it('rejects a risk body that does not match the contract', async () => {
    const client = createApiClient({ baseUrl: '', fetch: async () => json({ score: 1, risk_level: 'x' }) });
    await expect(client.getRisk({ lat: 0, lon: 0 })).rejects.toBeInstanceOf(MalformedResponseError);
  });

  it('accepts assessment metadata and rejects a malformed confidence', async () => {
    const withMeta = {
      ...risk,
      stale: false,
      source_timestamps: { weather_fetched_at: '2026-10-07T12:00:00Z' },
      confidence: { level: 'medium', reasons: ['Not yet validated.'] },
      model_version: 'heuristic-1',
    };
    const ok = createApiClient({ baseUrl: '', fetch: async () => json(withMeta) });
    expect((await ok.getRisk({ lat: 0, lon: 0 })).confidence?.level).toBe('medium');

    const bad = createApiClient({ baseUrl: '', fetch: async () => json({ ...withMeta, confidence: { level: 'certain' } }) });
    await expect(bad.getRisk({ lat: 0, lon: 0 })).rejects.toBeInstanceOf(MalformedResponseError);
  });

  describe('device weather fallback', () => {
    const weather = { latitude: -0.1, longitude: 34.8, current: {}, hourly: {} };
    const unavailable = () => json({ error: 'weather data is temporarily unavailable' }, 503);

    it('fetches weather on the device and posts it when the server answers 503', async () => {
      const fetch = jest.fn(async (url: string) => {
        if (url.startsWith('https://api.open-meteo.com/')) return json(weather);
        return url.endsWith('/api/risk') ? json(risk) : unavailable();
      });
      const client = createApiClient({ baseUrl: 'http://api.test', fetch });

      const result = await client.getRisk({ name: 'Kisumu', lat: -0.07, lon: 34.77 });

      expect(result.location.name).toBe('Kisumu');
      const weatherUrl = new URL(fetch.mock.calls[1][0]);
      expect(Object.fromEntries(weatherUrl.searchParams)).toMatchObject({
        latitude: '-0.070000',
        longitude: '34.770000',
        past_days: '7',
        forecast_days: '2',
        hourly: 'precipitation,rain,soil_moisture_0_to_7cm',
        timezone: 'auto',
      });
      expect(fetch.mock.calls[2]).toEqual([
        'http://api.test/api/risk',
        expect.objectContaining({ method: 'POST', body: JSON.stringify({ lat: -0.07, lon: 34.77, weather }) }),
      ]);
    });

    it('throws the server error when the device cannot get weather either', async () => {
      const fetch = jest.fn(async (url: string) =>
        url.startsWith('https://api.open-meteo.com/') ? json({ reason: 'limit' }, 429) : unavailable()
      );
      const client = createApiClient({ baseUrl: '', fetch });
      await expect(client.getRisk({ lat: 0, lon: 0 })).rejects.toMatchObject({ status: 503 });
      expect(fetch).toHaveBeenCalledTimes(2);
    });

    it('is skipped for other errors and when disabled', async () => {
      const notFound = jest.fn(async () => json({ error: 'nope' }, 400));
      await expect(createApiClient({ baseUrl: '', fetch: notFound }).getRisk({ lat: 0, lon: 0 })).rejects.toMatchObject({
        status: 400,
      });
      expect(notFound).toHaveBeenCalledTimes(1);

      const down = jest.fn(async () => unavailable());
      const client = createApiClient({ baseUrl: '', fetch: down, weatherFallback: false });
      await expect(client.getRisk({ lat: 0, lon: 0 })).rejects.toMatchObject({ status: 503 });
      expect(down).toHaveBeenCalledTimes(1);
    });
  });

  it('validates a report before sending it', async () => {
    const fetch = jest.fn();
    const client = createApiClient({ baseUrl: '', fetch });

    await expect(
      client.createReport({ location: { name: 'X', lat: 200, lon: 0 }, category: 'flooding', note: 'n' })
    ).rejects.toBeInstanceOf(ValidationError);
    expect(fetch).not.toHaveBeenCalled();
  });

  it('posts the normalised report', async () => {
    const created = {
      id: 'r1',
      location: { name: 'Bridge', lat: 1, lon: 2 },
      category: 'flooding',
      note: 'Water',
      timestamp: '2026-10-06T10:00:00Z',
      photos: [],
    };
    const fetch = jest.fn(async () => json(created, 201));
    const client = createApiClient({ baseUrl: '', fetch });

    await expect(
      client.createReport({ location: { name: ' Bridge ', lat: 1, lon: 2 }, category: 'flooding', note: ' Water ' })
    ).resolves.toEqual(created);
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/reports');
    expect(JSON.parse(init.body as string)).toEqual({
      location: { name: 'Bridge', lat: 1, lon: 2 },
      category: 'flooding',
      note: 'Water',
    });
  });
});
