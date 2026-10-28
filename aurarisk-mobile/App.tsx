import type { Location } from '@aurarisk/shared';
import { StatusBar } from 'expo-status-bar';
import { useCallback, useEffect, useState } from 'react';
import { ScrollView, StyleSheet, Text } from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { ensureSession } from './src/auth/authClient';
import { registerBackgroundRefreshAsync } from './src/background/backgroundRefresh';
import LocationPicker from './src/components/LocationPicker';
import NotificationSettings from './src/components/NotificationSettings';
import ReportComposer from './src/components/ReportComposer';
import { useOfflineSync } from './src/hooks/useOfflineSync';
import { setupNotifications } from './src/notifications/setup';
import LanguageToggle from './src/components/LanguageToggle';
import RiskPanel from './src/components/RiskPanel';
import { I18nProvider, useT } from './src/i18n';

export default function App() {
  return (
    <I18nProvider>
      <Main />
    </I18nProvider>
  );
}

function Main() {
  const { t } = useT();
  const { pendingCount, isSyncing, refreshPendingCount } = useOfflineSync();
  const [location, setLocation] = useState<Location | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const onReportSubmitted = useCallback(() => {
    refreshPendingCount();
    setRefreshKey((k) => k + 1);
  }, [refreshPendingCount]);

  useEffect(() => {
    ensureSession().catch((err) => console.warn('Device registration deferred:', err));
    registerBackgroundRefreshAsync().catch((err) => console.warn('Background refresh unavailable:', err));
    return setupNotifications();
  }, []);

  return (
    <SafeAreaProvider>
      <SafeAreaView style={styles.safe}>
        <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
          <LanguageToggle />
          {(pendingCount > 0 || isSyncing) && (
            <Text style={styles.banner}>
              {isSyncing ? t('mobile.sendingQueued') : t('mobile.waiting', { count: pendingCount })}
            </Text>
          )}
          <LocationPicker value={location} onChange={setLocation} />
          {location && <RiskPanel location={location} refreshKey={refreshKey} />}
          <ReportComposer location={location} onSubmitted={onReportSubmitted} />
          <NotificationSettings />
        </ScrollView>
        <StatusBar style="auto" />
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: '#fff' },
  container: { padding: 16, gap: 32 },
  banner: { backgroundColor: '#fff4ce', padding: 10, borderRadius: 6 },
});
