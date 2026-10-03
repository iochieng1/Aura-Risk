import * as Notifications from 'expo-notifications';
import { onSessionReset } from '../auth/authClient';
import { fetchRiskAssessment } from '../services/api';
import { RiskAlertData } from '../types/api';
import { ensureAndroidChannel, syncPushRegistration } from './push';
import { restoreCachedSubscriptions } from './subscriptions';

// Module scope so it is in place before any notification arrives, including after a cold start.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

const isRiskAlert = (data: unknown): data is RiskAlertData => {
  if (!data || typeof data !== 'object') return false;
  const d = data as Record<string, unknown>;
  return d.type === 'risk_alert' && typeof d.lat === 'number' && typeof d.lon === 'number';
};

export type RiskAlertHandler = (alert: RiskAlertData) => void;

/**
 * Wires notification listeners. Returns a cleanup function. `onRiskAlertOpened` is called when the
 * user taps a risk alert, after the cached risk for that location has been refreshed.
 */
export const setupNotifications = (onRiskAlertOpened?: RiskAlertHandler): (() => void) => {
  ensureAndroidChannel().catch((err) => console.warn('Failed to create notification channel:', err));
  syncPushRegistration().catch((err) => console.warn('Push registration sync failed:', err));

  const handleResponse = async (response: Notifications.NotificationResponse) => {
    const data = response.notification.request.content.data;
    if (!isRiskAlert(data)) return;
    console.log(`Risk alert opened: subscription ${data.subscription_id} level ${data.level}`);
    await fetchRiskAssessment(data.lat, data.lon);
    onRiskAlertOpened?.(data);
  };

  const responseSubscription = Notifications.addNotificationResponseReceivedListener((response) => {
    handleResponse(response).catch((err) => console.warn('Failed to handle notification tap:', err));
  });

  // A tap that cold-started the app happened before the listener existed.
  const last = Notifications.getLastNotificationResponse();
  if (last) {
    handleResponse(last).catch((err) => console.warn('Failed to handle notification tap:', err));
    Notifications.clearLastNotificationResponse();
  }

  // Expo push tokens can rotate; re-register when the native token changes.
  const tokenSubscription = Notifications.addPushTokenListener(() => {
    syncPushRegistration().catch((err) => console.warn('Push token resync failed:', err));
  });

  const removeResetHandler = onSessionReset(async () => {
    await syncPushRegistration();
    await restoreCachedSubscriptions();
  });

  return () => {
    responseSubscription.remove();
    tokenSubscription.remove();
    removeResetHandler();
  };
};
