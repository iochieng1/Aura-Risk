import * as SecureStore from 'expo-secure-store';
import { TokenPair } from '../types/api';

// Tokens live only in the platform keystore (iOS Keychain / Android Keystore), never AsyncStorage.
const SESSION_KEY = 'aurarisk.session';

// AFTER_FIRST_UNLOCK so the background refresh task can read tokens while the phone is locked;
// THIS_DEVICE_ONLY so tokens are never migrated to another device through a backup restore.
const OPTIONS: SecureStore.SecureStoreOptions = {
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY,
};

const isTokenPair = (value: unknown): value is TokenPair => {
  if (!value || typeof value !== 'object') return false;
  const v = value as Record<string, unknown>;
  return ['account_id', 'device_id', 'access_token', 'access_expires_at', 'refresh_token', 'refresh_expires_at'].every(
    (key) => typeof v[key] === 'string' && (v[key] as string).length > 0
  );
};

export const loadSession = async (): Promise<TokenPair | null> => {
  try {
    const raw = await SecureStore.getItemAsync(SESSION_KEY, OPTIONS);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return isTokenPair(parsed) ? parsed : null;
  } catch {
    return null;
  }
};

export const saveSession = async (session: TokenPair): Promise<void> => {
  // Stored as one entry so access and refresh tokens are always written together.
  await SecureStore.setItemAsync(SESSION_KEY, JSON.stringify(session), OPTIONS);
};

export const clearSession = async (): Promise<void> => {
  await SecureStore.deleteItemAsync(SESSION_KEY, OPTIONS);
};
