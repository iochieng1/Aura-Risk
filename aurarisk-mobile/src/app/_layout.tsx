import { router, Slot } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { ensureSession } from '../auth/authClient';
import { registerBackgroundRefreshAsync } from '../background/backgroundRefresh';
import { warmUpServer } from '../config';
import { I18nProvider } from '../i18n';
import { riskAlertHref } from '../navigation/riskLink';
import { fetchWithRetry } from '../net/fetchWithRetry';
import { setupNotifications } from '../notifications/setup';
import { pruneOrphanPhotos } from '../photos/photoStore';
import { AppStateProvider } from '../state/AppState';

/** App root: providers shared by every tab, and the one-time start-up work. */
export default function RootLayout() {
  useEffect(() => {
    warmUpServer(fetchWithRetry);
    ensureSession().catch((err) => console.warn('Device registration deferred:', err));
    registerBackgroundRefreshAsync().catch((err) => console.warn('Background refresh unavailable:', err));
    // Photos left behind by a report that was never submitted; queued reports' photos are kept.
    pruneOrphanPhotos().catch((err) => console.warn('Photo cleanup failed:', err));
    // A tapped risk alert opens the Risk tab for its location. Child effects run first, so the
    // navigator is mounted by the time a cold-start tap is handled here.
    return setupNotifications((alert) => router.navigate(riskAlertHref(alert)));
  }, []);

  return (
    <SafeAreaProvider>
      <I18nProvider>
        <AppStateProvider>
          <Slot />
          <StatusBar style="auto" />
        </AppStateProvider>
      </I18nProvider>
    </SafeAreaProvider>
  );
}
