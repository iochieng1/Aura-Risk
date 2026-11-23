import { categoryLabel, type CommunityReport, type Location } from '@aurarisk/shared';
import { Camera, Map, Marker } from '@maplibre/maplibre-react-native';
import { useState } from 'react';
import { Linking, Pressable, StyleSheet, Text, View } from 'react-native';
import { useT } from '../i18n';
import { ReportSummary } from './ReportList';

/** Same style as the web map. Tiles the device has already shown are kept in MapLibre's ambient cache. */
export const MAP_STYLE_URL = 'https://tiles.openfreemap.org/styles/bright';
const OSM_COPYRIGHT_URL = 'https://www.openstreetmap.org/copyright';

const REPORT_ICONS: Record<CommunityReport['category'], string> = {
  flooding: '🌊',
  road_blocked: '🚧',
  water_rising: '📈',
  drainage_issue: '🕳️',
};

interface Props {
  location: Location;
  reports: CommunityReport[];
}

/** MapLibre map centred on the selected location, with a marker per nearby report. */
export default function ReportsMap({ location, reports }: Props) {
  const { lang, t } = useT();
  const [selected, setSelected] = useState<CommunityReport | null>(null);
  const shown = selected && reports.some((r) => r.id === selected.id) ? selected : null;

  return (
    <View style={styles.fill}>
      <Map
        style={styles.fill}
        mapStyle={MAP_STYLE_URL}
        // Attribution is drawn below as text so it is always visible and translated.
        attribution={false}
        logo={false}
        accessibilityLabel={t('mobile.mapLabel', { place: location.name, count: reports.length })}
      >
        <Camera center={[location.lon, location.lat]} zoom={13} />
        <Marker id="selected-location" lngLat={[location.lon, location.lat]}>
          <View
            style={styles.selectedPin}
            accessible
            accessibilityLabel={t('mobile.selectedLocation', { place: location.name })}
          />
        </Marker>
        {reports.map((r) => (
          <Marker key={r.id} id={r.id} lngLat={[r.location.lon, r.location.lat]}>
            <Pressable
              onPress={() => setSelected(r)}
              accessibilityRole="button"
              accessibilityLabel={t('mobile.reportMarker', { category: categoryLabel(lang, r.category) })}
              hitSlop={8}
            >
              <Text style={styles.reportIcon}>{REPORT_ICONS[r.category] ?? '📌'}</Text>
            </Pressable>
          </Marker>
        ))}
      </Map>

      {shown && (
        <View style={styles.card} accessibilityViewIsModal>
          <ReportSummary report={shown} />
          <Pressable onPress={() => setSelected(null)} accessibilityRole="button" style={styles.close}>
            <Text style={styles.closeText}>{t('mobile.close')}</Text>
          </Pressable>
        </View>
      )}

      <Text
        style={styles.attribution}
        accessibilityRole="link"
        onPress={() => Linking.openURL(OSM_COPYRIGHT_URL)}
      >
        {t('mobile.mapAttribution')}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
  selectedPin: {
    width: 18,
    height: 18,
    borderRadius: 9,
    backgroundColor: '#0b5cad',
    borderWidth: 3,
    borderColor: '#fff',
  },
  reportIcon: { fontSize: 24 },
  card: {
    position: 'absolute',
    left: 12,
    right: 12,
    bottom: 36,
    backgroundColor: '#fff',
    borderRadius: 8,
    padding: 12,
    gap: 8,
    elevation: 4,
    shadowColor: '#000',
    shadowOpacity: 0.2,
    shadowRadius: 6,
    shadowOffset: { width: 0, height: 2 },
  },
  close: { alignSelf: 'flex-end', paddingVertical: 6, paddingHorizontal: 12 },
  closeText: { color: '#0b5cad', fontWeight: '600' },
  attribution: {
    position: 'absolute',
    right: 0,
    bottom: 0,
    fontSize: 11,
    color: '#333',
    backgroundColor: 'rgba(255,255,255,0.8)',
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
});
