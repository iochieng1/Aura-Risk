import AsyncStorage from '@react-native-async-storage/async-storage';
import Constants from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { Platform } from 'react-native';
import { authFetch } from '../auth/authClient';

/** Must match the channelId the backend sets on Expo push messages. */
export const RISK_ALERT_CHANNEL_ID = 'risk-alerts';

// Records that the user said yes in our own explanation screen. Not a secret; the push token itself
// is only sent to our backend.
const CONSENT_KEY = '@aurarisk/push_consent';

interface StoredConsent {
  consentedAt: string;
  pushToken: string;
}

export type EnablePushResult =
  | { status: 'enabled'; consentedAt: string }
  | { status: 'denied' } // OS permission refused
  | { status: 'unavailable'; reason: string };

export const ensureAndroidChannel = async (): Promise<void> => {
  if (Platform.OS !== 'android') return;
  // Android 13+ won't show the permission prompt until a channel exists.
  await Notifications.setNotificationChannelAsync(RISK_ALERT_CHANNEL_ID, {
    name: 'Flood risk alerts',
    description: 'Alerts when flood risk rises at locations you follow',
    importance: Notifications.AndroidImportance.HIGH,
    vibrationPattern: [0, 250, 250, 250],
  });
};

export const getStoredConsent = async (): Promise<StoredConsent | null> => {
  try {
    const raw = await AsyncStorage.getItem(CONSENT_KEY);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
};

const isPermissionGranted = (settings: Notifications.NotificationPermissionsStatus) =>
  settings.granted ||
  settings.ios?.status === Notifications.IosAuthorizationStatus.AUTHORIZED ||
  settings.ios?.status === Notifications.IosAuthorizationStatus.PROVISIONAL;

const getProjectId = (): string | undefined =>
  Constants.expoConfig?.extra?.eas?.projectId ?? Constants.easConfig?.projectId;

const registerTokenWithBackend = async (pushToken: string): Promise<string> => {
  const response = await authFetch('/api/devices/me/push', {
    method: 'PUT',
    body: JSON.stringify({ push_token: pushToken, consent: true }),
  });
  if (!response.ok) throw new Error(`Push registration failed (${response.status})`);
  const body: { push_enabled: boolean; consented_at: string } = await response.json();
  await AsyncStorage.setItem(CONSENT_KEY, JSON.stringify({ consentedAt: body.consented_at, pushToken }));
  return body.consented_at;
};

/**
 * Call ONLY after the user has agreed in the in-app explanation. Requests the OS permission, then
 * fetches the Expo push token and registers it with explicit consent.
 */
export const enablePushNotifications = async (): Promise<EnablePushResult> => {
  if (!Device.isDevice) {
    return { status: 'unavailable', reason: 'Push notifications need a physical device.' };
  }
  const projectId = getProjectId();
  if (!projectId) {
    return { status: 'unavailable', reason: 'EAS projectId is not configured (app.json extra.eas.projectId).' };
  }

  await ensureAndroidChannel();

  let settings = await Notifications.getPermissionsAsync();
  if (!isPermissionGranted(settings)) {
    if (!settings.canAskAgain) return { status: 'denied' };
    settings = await Notifications.requestPermissionsAsync({
      ios: { allowAlert: true, allowSound: true, allowBadge: false },
    });
  }
  if (!isPermissionGranted(settings)) return { status: 'denied' };

  try {
    const { data: pushToken } = await Notifications.getExpoPushTokenAsync({ projectId });
    const consentedAt = await registerTokenWithBackend(pushToken);
    return { status: 'enabled', consentedAt };
  } catch (err) {
    return { status: 'unavailable', reason: err instanceof Error ? err.message : String(err) };
  }
};

/** Withdraws consent server-side and forgets it locally. */
export const disablePushNotifications = async (): Promise<void> => {
  const response = await authFetch('/api/devices/me/push', { method: 'DELETE' });
  if (!response.ok && response.status !== 404) {
    throw new Error(`Failed to withdraw push consent (${response.status})`);
  }
  await AsyncStorage.removeItem(CONSENT_KEY);
};

/**
 * Re-sends the push token when it may have changed (token rotation, or a new device account).
 * Never prompts: it only acts when consent was given before and the OS permission still stands.
 * If the user revoked the permission in system settings, consent is withdrawn server-side too.
 */
export const syncPushRegistration = async (): Promise<void> => {
  const consent = await getStoredConsent();
  if (!consent) return;

  const settings = await Notifications.getPermissionsAsync();
  if (!isPermissionGranted(settings)) {
    await disablePushNotifications().catch(() => undefined);
    return;
  }

  const projectId = getProjectId();
  if (!projectId) return;
  const { data: token } = await Notifications.getExpoPushTokenAsync({ projectId });
  await registerTokenWithBackend(token);
};

export const clearLocalPushConsent = () => AsyncStorage.removeItem(CONSENT_KEY);
