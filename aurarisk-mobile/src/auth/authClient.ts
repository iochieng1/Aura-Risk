import AsyncStorage from '@react-native-async-storage/async-storage';
import Constants from 'expo-constants';
import { Platform } from 'react-native';
import { API_BASE_URL, fetchWithTimeout } from '../config';
import { TokenPair } from '../types/api';
import { clearSession, loadSession, saveSession } from './tokenStore';

// Refresh a little before expiry so requests don't race the deadline.
const EXPIRY_SKEW_MS = 60_000;

// The iOS Keychain survives uninstall. AsyncStorage does not, so a missing marker means a fresh
// install and any session left in the Keychain belongs to a previous install.
const INSTALL_MARKER_KEY = '@aurarisk/install_marker';

export class AuthError extends Error {}

type SessionResetHandler = () => Promise<void> | void;
const sessionResetHandlers = new Set<SessionResetHandler>();

/**
 * Called after the app had to register a brand-new device account (refresh token rejected or
 * reused). Server-side state tied to the old account (push token, subscriptions) is gone.
 */
export const onSessionReset = (handler: SessionResetHandler): (() => void) => {
  sessionResetHandlers.add(handler);
  return () => sessionResetHandlers.delete(handler);
};

let cachedSession: TokenPair | null = null;
let sessionLoaded = false;
let inFlightRegister: Promise<TokenPair> | null = null;
let inFlightRefresh: Promise<TokenPair> | null = null;

const isExpired = (iso: string, skewMs = 0) => {
  const t = Date.parse(iso);
  return Number.isNaN(t) || t - skewMs <= Date.now();
};

const persist = async (session: TokenPair) => {
  cachedSession = session;
  await saveSession(session);
  return session;
};

const readSession = async (): Promise<TokenPair | null> => {
  if (sessionLoaded) return cachedSession;
  const marker = await AsyncStorage.getItem(INSTALL_MARKER_KEY).catch(() => null);
  if (!marker) {
    await clearSession().catch(() => undefined);
    await AsyncStorage.setItem(INSTALL_MARKER_KEY, '1').catch(() => undefined);
    cachedSession = null;
  } else {
    cachedSession = await loadSession();
  }
  sessionLoaded = true;
  return cachedSession;
};

const parseTokenPair = async (response: Response): Promise<TokenPair> => {
  const body = (await response.json()) as TokenPair;
  if (!body?.access_token || !body?.refresh_token) throw new AuthError('Malformed token response');
  return body;
};

const registerDevice = (): Promise<TokenPair> => {
  if (inFlightRegister) return inFlightRegister;
  inFlightRegister = (async () => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/api/auth/device`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        platform: Platform.OS === 'ios' || Platform.OS === 'android' ? Platform.OS : 'web',
        app_version: Constants.expoConfig?.version,
      }),
    });
    if (response.status !== 201 && !response.ok) {
      throw new AuthError(`Device registration failed (${response.status})`);
    }
    return persist(await parseTokenPair(response));
  })().finally(() => {
    inFlightRegister = null;
  });
  return inFlightRegister;
};

const resetAndRegister = async (): Promise<TokenPair> => {
  cachedSession = null;
  await clearSession().catch(() => undefined);
  const session = await registerDevice();
  for (const handler of sessionResetHandlers) {
    try {
      await handler();
    } catch (err) {
      console.warn('Session reset handler failed:', err);
    }
  }
  return session;
};

/**
 * Rotates the refresh token. Concurrent callers share one request so a rotated token is never
 * presented twice (the server treats reuse as theft and revokes the device).
 */
export const refreshSession = (): Promise<TokenPair> => {
  if (inFlightRefresh) return inFlightRefresh;
  inFlightRefresh = (async () => {
    const current = await readSession();
    if (!current || isExpired(current.refresh_expires_at)) return resetAndRegister();

    const response = await fetchWithTimeout(`${API_BASE_URL}/api/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: current.refresh_token }),
    });
    // 401 means the refresh token is invalid, expired, or was reused: start over as a new device.
    // Any other failure (5xx, network) keeps the session so we can retry later.
    if (response.status === 401) return resetAndRegister();
    if (!response.ok) throw new AuthError(`Token refresh failed (${response.status})`);
    return persist(await parseTokenPair(response));
  })().finally(() => {
    inFlightRefresh = null;
  });
  return inFlightRefresh;
};

/** Returns a session, registering this device on first use. */
export const ensureSession = async (): Promise<TokenPair> => {
  const session = await readSession();
  if (!session) return registerDevice();
  return session;
};

export const getAccessToken = async (): Promise<string> => {
  let session = await ensureSession();
  if (isExpired(session.access_expires_at, EXPIRY_SKEW_MS)) {
    session = await refreshSession();
  }
  return session.access_token;
};

/** fetch() for authenticated API calls. Retries once with fresh tokens on 401. */
export const authFetch = async (path: string, init: RequestInit = {}): Promise<Response> => {
  const send = async (token: string) => {
    const headers = new Headers(init.headers);
    headers.set('Authorization', `Bearer ${token}`);
    if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
    return fetchWithTimeout(`${API_BASE_URL}${path}`, { ...init, headers });
  };

  const response = await send(await getAccessToken());
  if (response.status !== 401) return response;

  const refreshed = await refreshSession();
  return send(refreshed.access_token);
};

export const getSessionInfo = async (): Promise<{ accountId: string; deviceId: string } | null> => {
  const session = await readSession();
  return session ? { accountId: session.account_id, deviceId: session.device_id } : null;
};

/** Revokes this device's tokens on the server (best effort) and forgets them locally. */
export const logout = async (): Promise<void> => {
  const session = await readSession();
  if (session) {
    try {
      await fetchWithTimeout(`${API_BASE_URL}/api/auth/logout`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${session.access_token}` },
      });
    } catch {
      // Tokens expire on their own; local removal is what matters here.
    }
  }
  cachedSession = null;
  await clearSession();
};

/** Deletes the account server-side, then all local credentials. Throws if the server refuses. */
export const deleteAccount = async (): Promise<void> => {
  const response = await authFetch('/api/account', { method: 'DELETE' });
  if (response.status !== 204 && !response.ok) {
    throw new AuthError(`Account deletion failed (${response.status})`);
  }
  cachedSession = null;
  await clearSession();
};

/** Test-only: forget in-memory state between test cases. */
export const __resetAuthStateForTests = () => {
  cachedSession = null;
  sessionLoaded = false;
  inFlightRegister = null;
  inFlightRefresh = null;
  sessionResetHandlers.clear();
};
