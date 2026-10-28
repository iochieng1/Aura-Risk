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
      model_version: 'heuristic-2',
    };
    const ok = createApiClient({ baseUrl: '', fetch: async () => json(withMeta) });
    expect((await ok.getRisk({ lat: 0, lon: 0 })).confidence?.level).toBe('medium');

    const bad = createApiClient({ baseUrl: '', fetch: async () => json({ ...withMeta, confidence: { level: 'certain' } }) });
    await expect(bad.getRisk({ lat: 0, lon: 0 })).rejects.toBeInstanceOf(MalformedResponseError);
  });

  it('accepts partial hydrology and rejects non-numeric values', async () => {
    const withHydrology = { ...risk, hydrology: { elevation_m: 1151, river_peak_m3s: 60.9 } };
    const ok = createApiClient({ baseUrl: '', fetch: async () => json(withHydrology) });
    expect((await ok.getRisk({ lat: 0, lon: 0 })).hydrology?.river_peak_m3s).toBe(60.9);

    const bad = createApiClient({ baseUrl: '', fetch: async () => json({ ...risk, hydrology: { elevation_m: '1151' } }) });
    await expect(bad.getRisk({ lat: 0, lon: 0 })).rejects.toBeInstanceOf(MalformedResponseError);
  });

  describe('device weather fallback', () => {
    const weather = { latitude: -0.1, longitude: 34.8, current: {}, hourly: {} };
    const unavailable = () => json({ error: 'weather data is temporarily unavailable' }, 503);

    const elevation = { elevation: [1140, 1150, 1150, 1150, 1150, 1150, 1150, 1150, 1150] };
    const flood = [{ latitude: -0.075, longitude: 34.775, daily: { time: [], river_discharge: [] } }];
    const openMeteo = (url: string) => {
      if (url.startsWith('https://api.open-meteo.com/v1/forecast')) return json(weather);
      if (url.startsWith('https://api.open-meteo.com/v1/elevation')) return json(elevation);
      if (url.startsWith('https://flood-api.open-meteo.com/')) return json(flood);
      return undefined;
    };
    const posted = (fetch: jest.Mock) => {
      const call = fetch.mock.calls.find(([url, init]) => url.includes('/api/risk') && init?.method === 'POST');
      return call && JSON.parse(call[1].body);
    };

    it('fetches weather, elevation and river data on the device and posts them when the server answers 503', async () => {
      const fetch = jest.fn(async (url: string) => openMeteo(url) ?? (url.endsWith('/api/risk') ? json(risk) : unavailable()));
      const client = createApiClient({ baseUrl: 'http://api.test', fetch });

      const result = await client.getRisk({ name: 'Kisumu', lat: -0.07, lon: 34.77 });

      expect(result.location.name).toBe('Kisumu');
      const urls = fetch.mock.calls.map(([url]) => new URL(url));
      const forecast = urls.find((u) => u.pathname === '/v1/forecast')!;
      expect(Object.fromEntries(forecast.searchParams)).toMatchObject({
        latitude: '-0.070000',
        longitude: '34.770000',
        past_days: '7',
        forecast_days: '2',
        hourly: 'precipitation,rain,soil_moisture_0_to_7cm',
        timezone: 'auto',
      });
      // Same points, in the same order, as the backend's neighbourhood().
      const elevationUrl = urls.find((u) => u.pathname === '/v1/elevation')!;
      expect(elevationUrl.searchParams.get('latitude')).toBe(
        '-0.0700,-0.0800,-0.0800,-0.0800,-0.0700,-0.0700,-0.0600,-0.0600,-0.0600'
      );
      expect(elevationUrl.searchParams.get('longitude')).toBe(
        '34.7700,34.7600,34.7700,34.7800,34.7600,34.7800,34.7600,34.7700,34.7800'
      );
      const floodUrl = urls.find((u) => u.pathname === '/v1/flood')!;
      expect(Object.fromEntries(floodUrl.searchParams)).toMatchObject({
        daily: 'river_discharge',
        past_days: '92',
        forecast_days: '7',
      });
      expect(floodUrl.searchParams.get('latitude')?.split(',')[1]).toBe('-0.1200');
      expect(posted(fetch)).toEqual({ lat: -0.07, lon: 34.77, weather, elevation, flood });
    });

    it('still posts the weather when elevation and river data fail', async () => {
      const fetch = jest.fn(async (url: string) => {
        if (url.startsWith('https://api.open-meteo.com/v1/forecast')) return json(weather);
        if (url.includes('open-meteo.com')) return json({ reason: 'limit' }, 429);
        return url.endsWith('/api/risk') ? json(risk) : unavailable();
      });
      await createApiClient({ baseUrl: 'http://api.test', fetch }).getRisk({ lat: -0.07, lon: 34.77 });
      expect(posted(fetch)).toEqual({ lat: -0.07, lon: 34.77, weather });
    });

    it('keeps the language on the fallback', async () => {
      const fetch = jest.fn(async (url: string) => openMeteo(url) ?? (url.includes('/api/risk?lang=') ? json(risk) : unavailable()));
      const client = createApiClient({ baseUrl: 'http://api.test', fetch, lang: () => 'sw' });

      await client.getRisk({ lat: -0.07, lon: 34.77 });

      expect(fetch.mock.calls[0][0]).toBe('http://api.test/api/risk?lat=-0.07&lon=34.77&lang=sw');
      expect(fetch.mock.calls.at(-1)[0]).toBe('http://api.test/api/risk?lang=sw');
    });

    it('throws the server error when the device cannot get weather either', async () => {
      const fetch = jest.fn(async (url: string) =>
        url.includes('open-meteo.com') ? json({ reason: 'limit' }, 429) : unavailable()
      );
      const client = createApiClient({ baseUrl: '', fetch });
      await expect(client.getRisk({ lat: 0, lon: 0 })).rejects.toMatchObject({ status: 503 });
      // The server, then weather, elevation and flood on the device; no POST.
      expect(fetch).toHaveBeenCalledTimes(4);
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

describe('county stats and open data', () => {
  it('passes the period and builds export links', async () => {
    const fetch = jest.fn(async () => json({ from: '2026-01-01', to: '2026-03-31', counties: [] }));
    const client = createApiClient({ baseUrl: 'http://api.test', fetch });

    const stats = await client.getCountyStats({ from: '2026-01-01', to: '2026-03-31' });

    expect(fetch).toHaveBeenCalledWith('http://api.test/api/stats/counties?from=2026-01-01&to=2026-03-31', undefined);
    expect(stats.counties).toEqual([]);
    expect(client.openDataUrl('csv', { county: 'Homa Bay' })).toBe(
      'http://api.test/api/open-data/reports.csv?county=Homa%20Bay'
    );
    expect(client.openDataUrl('json')).toBe('http://api.test/api/open-data/reports.json');
  });

  it('rejects a malformed stats body', async () => {
    const client = createApiClient({ baseUrl: '', fetch: async () => json({ counties: 'none' }) });
    await expect(client.getCountyStats()).rejects.toBeInstanceOf(MalformedResponseError);
  });
});
