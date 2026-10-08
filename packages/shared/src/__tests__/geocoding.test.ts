import { createPlaceSearch, PLACE_SEARCH_MIN_INTERVAL_MS } from '../geocoding';

const json = (body: unknown, status = 200) =>
  ({ ok: status >= 200 && status < 300, status, json: async () => body }) as Response;

const nairobi = [{ display_name: 'Nairobi, Kenya', lat: '-1.29', lon: '36.82' }];

function setup() {
  let clock = 0;
  const sleeps: number[] = [];
  const search = createPlaceSearch({
    now: () => clock,
    sleep: async (ms) => {
      sleeps.push(ms);
    },
  });
  const fetch = jest.fn(async () => json(nairobi));
  return { search, fetch, sleeps, advance: (ms: number) => (clock += ms) };
}

describe('searchPlaces', () => {
  it('maps results and sends the User-Agent', async () => {
    const { search, fetch } = setup();
    const places = await search('Nairobi', { fetch, userAgent: 'AuraRisk-test' });
    expect(places).toEqual([{ name: 'Nairobi, Kenya', lat: -1.29, lon: 36.82 }]);
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining('q=Nairobi'), {
      headers: { 'Accept-Language': 'en', 'User-Agent': 'AuraRisk-test' },
    });
  });

  it('skips short queries without calling Nominatim', async () => {
    const { search, fetch } = setup();
    expect(await search(' ab ', { fetch })).toEqual([]);
    expect(fetch).not.toHaveBeenCalled();
  });

  it('caches repeated queries', async () => {
    const { search, fetch } = setup();
    await search('Nairobi', { fetch });
    await search('  nairobi ', { fetch });
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('does not cache failures', async () => {
    const { search, advance } = setup();
    const fetch = jest.fn(async () => json({}, 503));
    await search('Nairobi', { fetch });
    advance(PLACE_SEARCH_MIN_INTERVAL_MS);
    await search('Nairobi', { fetch });
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('allows at most one request per second', async () => {
    const { search, fetch, sleeps, advance } = setup();
    await Promise.all([search('Nairobi', { fetch }), search('Mombasa', { fetch }), search('Kisumu', { fetch })]);
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(sleeps).toEqual([PLACE_SEARCH_MIN_INTERVAL_MS, 2 * PLACE_SEARCH_MIN_INTERVAL_MS]);

    // After a quiet period the next search goes straight through.
    advance(5 * PLACE_SEARCH_MIN_INTERVAL_MS);
    await search('Nakuru', { fetch });
    expect(sleeps).toHaveLength(2);
  });
});
