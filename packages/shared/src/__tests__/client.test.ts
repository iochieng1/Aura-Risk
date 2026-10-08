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
