import * as SecureStore from 'expo-secure-store';
import { clearSession, loadSession, saveSession } from '../tokenStore';

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

const session = {
  account_id: 'acc-1',
  device_id: 'dev-1',
  access_token: 'access',
  access_expires_at: '2030-01-01T00:00:00Z',
  refresh_token: 'refresh',
  refresh_expires_at: '2030-02-01T00:00:00Z',
};

describe('tokenStore', () => {
  beforeEach(() => {
    jest.requireMock('expo-secure-store').__store.clear();
    jest.clearAllMocks();
  });

  it('round-trips a session through SecureStore with device-only, after-first-unlock access', async () => {
    await saveSession(session);
    expect(await loadSession()).toEqual(session);
    expect(SecureStore.setItemAsync).toHaveBeenCalledWith('aurarisk.session', JSON.stringify(session), {
      keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY,
    });
  });

  it('returns null when nothing is stored', async () => {
    expect(await loadSession()).toBeNull();
  });

  it('rejects corrupt or incomplete entries', async () => {
    const store: Map<string, string> = jest.requireMock('expo-secure-store').__store;
    store.set('aurarisk.session', 'not json');
    expect(await loadSession()).toBeNull();
    store.set('aurarisk.session', JSON.stringify({ ...session, refresh_token: '' }));
    expect(await loadSession()).toBeNull();
  });

  it('clears the session', async () => {
    await saveSession(session);
    await clearSession();
    expect(await loadSession()).toBeNull();
  });
});
