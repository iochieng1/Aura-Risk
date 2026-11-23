import { router } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import ReportList from '../../components/ReportList';
import ReportsMap from '../../components/ReportsMap';
import Screen from '../../components/Screen';
import { useNearbyReports } from '../../hooks/useNearbyReports';
import { useT } from '../../i18n';
import { useAppState } from '../../state/AppState';

type View_ = 'map' | 'list';

/** Map tab: nearby reports on a map, or the same reports as a list. */
export default function MapScreen() {
  const { t } = useT();
  const { location, refreshKey } = useAppState();
  const { reports, isOffline, loading } = useNearbyReports(location, refreshKey);
  const [view, setView] = useState<View_>('map');

  if (!location) {
    return (
      <Screen>
        <Text style={styles.body}>{t('mobile.mapNoLocation')}</Text>
        <Pressable style={styles.button} onPress={() => router.navigate('/')} accessibilityRole="button">
          <Text style={styles.buttonText}>{t('mobile.chooseLocation')}</Text>
        </Pressable>
      </Screen>
    );
  }

  return (
    <Screen scroll={false}>
      <View style={styles.header}>
        <Text style={styles.heading} accessibilityRole="header">
          {t('mobile.mapHeading')}
        </Text>
        <View style={styles.toggle} accessibilityRole="radiogroup" accessibilityLabel={t('mobile.showAs')}>
          {(['map', 'list'] as const).map((v) => (
            <Pressable
              key={v}
              onPress={() => setView(v)}
              style={[styles.chip, view === v && styles.chipSelected]}
              accessibilityRole="radio"
              accessibilityState={{ selected: view === v }}
            >
              <Text style={view === v ? styles.chipTextSelected : undefined}>
                {t(v === 'map' ? 'mobile.mapView' : 'mobile.listView')}
              </Text>
            </Pressable>
          ))}
        </View>
        {isOffline && (
          <Text style={styles.banner} accessibilityLiveRegion="polite">
            {t(view === 'map' ? 'mobile.mapOffline' : 'mobile.mapOfflineList')}
          </Text>
        )}
        {loading && <ActivityIndicator accessibilityLabel={t('mobile.loadingReports')} />}
      </View>

      {view === 'map' ? (
        <ReportsMap location={location} reports={reports} />
      ) : (
        <ScrollView contentContainerStyle={styles.list}>
          <Text style={styles.meta}>{t('mobile.nearbyReports', { count: reports.length })}</Text>
          <ReportList reports={reports} />
        </ScrollView>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  header: { paddingHorizontal: 16, paddingVertical: 8, gap: 8 },
  heading: { fontSize: 20, fontWeight: '600' },
  body: { fontSize: 15, lineHeight: 21 },
  meta: { fontSize: 13, color: '#555' },
  banner: { backgroundColor: '#fff4ce', padding: 10, borderRadius: 6 },
  toggle: { flexDirection: 'row', gap: 8 },
  chip: { paddingVertical: 4, paddingHorizontal: 12, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  chipSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  chipTextSelected: { color: '#fff' },
  list: { paddingHorizontal: 16, paddingBottom: 16, gap: 4 },
  button: { alignSelf: 'flex-start', backgroundColor: '#0b5cad', paddingVertical: 10, paddingHorizontal: 16, borderRadius: 6 },
  buttonText: { color: '#fff', fontWeight: '600' },
});
