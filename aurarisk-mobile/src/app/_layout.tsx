import type { RiskAlertData } from '@aurarisk/shared';
import { router, Slot, useRootNavigationState } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useState } from 'react';
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
  // A tapped risk alert waits here until the navigator is ready: a tap that cold-started the app
  // can be handled before the initial route has been resolved.
  const [openedAlert, setOpenedAlert] = useState<RiskAlertData | null>(null);
  const navigationReady = Boolean(useRootNavigationState()?.key);

  useEffect(() => {
    warmUpServer(fetchWithRetry);
    ensureSession().catch((err) => console.warn('Device registration deferred:', err));
    registerBackgroundRefreshAsync().catch((err) => console.warn('Background refresh unavailable:', err));
    // Photos left behind by a report that was never submitted; queued reports' photos are kept.
    pruneOrphanPhotos().catch((err) => console.warn('Photo cleanup failed:', err));
    return setupNotifications(setOpenedAlert);
  }, []);

  useEffect(() => {
    if (!navigationReady || !openedAlert) return;
    router.navigate(riskAlertHref(openedAlert));
    setOpenedAlert(null);
  }, [navigationReady, openedAlert]);

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
