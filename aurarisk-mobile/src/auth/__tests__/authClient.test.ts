import AsyncStorage from '@react-native-async-storage/async-storage';
import { __resetAuthStateForTests, authFetch, getAccessToken, onSessionReset, refreshSession } from '../authClient';
import { saveSession } from '../tokenStore';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

jest.mock('expo-secure-store', () => {
  const store = new Map<string, string>();
  return {
    AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY: 1,
    getItemAsync: jest.fn(async (key: string) => store.get(key) ?? null),
    setItemAsync: jest.fn(async (key: string, value: string) => void store.set(key, value)),
    deleteItemAsync: jest.fn(async (key: string) => void store.delete(key)),
    __store: store,
  };
});

jest.mock('expo-constants', () => ({ __esModule: true, default: { expoConfig: { version: '1.0.0' } } }));

const future = (ms: number) => new Date(Date.now() + ms).toISOString();
const past = () => new Date(Date.now() - 1000).toISOString();

const pair = (n: number, accessExpires = future(15 * 60_000)) => ({
  account_id: 'acc-1',
  device_id: 'dev-1',
  access_token: `access-${n}`,
  access_expires_at: accessExpires,
  refresh_token: `refresh-${n}`,
  refresh_expires_at: future(30 * 24 * 3600_000),
});

const jsonResponse = (status: number, body: unknown) =>
  ({ status, ok: status >= 200 && status < 300, json: async () => body }) as Response;

const fetchMock = jest.fn();
globalThis.fetch = fetchMock as unknown as typeof fetch;

const callsTo = (path: string) => fetchMock.mock.calls.filter(([url]) => String(url).endsWith(path));

describe('authClient', () => {
  beforeEach(async () => {
    __resetAuthStateForTests();
    jest.requireMock('expo-secure-store').__store.clear();
    await AsyncStorage.clear();
    // Mark as an existing install so the stored session is not discarded.
    await AsyncStorage.setItem('@aurarisk/install_marker', '1');
    fetchMock.mockReset();
  });

  it('registers the device on first use', async () => {
    await AsyncStorage.clear(); // fresh install
    fetchMock.mockResolvedValueOnce(jsonResponse(201, pair(1)));

    expect(await getAccessToken()).toBe('access-1');
    expect(callsTo('/api/auth/device')).toHaveLength(1);
  });

  it('discards a keychain session left over from a previous install', async () => {
    await saveSession(pair(0));
    await AsyncStorage.clear(); // no install marker
    fetchMock.mockResolvedValueOnce(jsonResponse(201, pair(1)));

    expect(await getAccessToken()).toBe('access-1');
  });

  it('refreshes only once for concurrent callers with an expired access token', async () => {
    await saveSession(pair(1, past()));
    let resolveRefresh: (r: Response) => void = () => undefined;
    fetchMock.mockImplementation(
      (url: string) =>
        new Promise<Response>((resolve) => {
          if (url.endsWith('/api/auth/refresh')) resolveRefresh = resolve;
        })
    );

    const tokens = Promise.all([getAccessToken(), getAccessToken(), getAccessToken()]);
    await new Promise((r) => setTimeout(r, 0));
    resolveRefresh(jsonResponse(200, pair(2)));

    expect(await tokens).toEqual(['access-2', 'access-2', 'access-2']);
    expect(callsTo('/api/auth/refresh')).toHaveLength(1);
    expect(JSON.parse(callsTo('/api/auth/refresh')[0][1].body)).toEqual({ refresh_token: 'refresh-1' });
  });

  it('re-registers and notifies listeners when the refresh token is rejected', async () => {
    await saveSession(pair(1, past()));
    const onReset = jest.fn();
    onSessionReset(onReset);
    fetchMock.mockImplementation(async (url: string) =>
      url.endsWith('/api/auth/refresh') ? jsonResponse(401, { error: 'reused' }) : jsonResponse(201, pair(9))
    );

    expect(await getAccessToken()).toBe('access-9');
    expect(callsTo('/api/auth/device')).toHaveLength(1);
    expect(onReset).toHaveBeenCalledTimes(1);
  });

  it('keeps the session when refresh fails for a transient reason', async () => {
    await saveSession(pair(1, past()));
    fetchMock.mockResolvedValue(jsonResponse(503, {}));

    await expect(refreshSession()).rejects.toThrow('503');
    expect(callsTo('/api/auth/device')).toHaveLength(0);
  });

  it('authFetch retries once with a fresh token after a 401', async () => {
    await saveSession(pair(1));
    fetchMock.mockImplementation(async (url: string, init: RequestInit) => {
      if (url.endsWith('/api/auth/refresh')) return jsonResponse(200, pair(2));
      const auth = new Headers(init.headers).get('Authorization');
      return auth === 'Bearer access-2' ? jsonResponse(200, { ok: true }) : jsonResponse(401, {});
    });

    const response = await authFetch('/api/subscriptions');

    expect(response.status).toBe(200);
    expect(callsTo('/api/auth/refresh')).toHaveLength(1);
    expect(callsTo('/api/subscriptions')).toHaveLength(2);
  });
});
