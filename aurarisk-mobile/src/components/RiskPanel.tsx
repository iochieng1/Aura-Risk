import {
  categoryLabel,
  type CommunityReport,
  describeFreshness,
  type Location,
  reportStatusLabel,
  RISK_LEVEL_COLORS,
  riskLevelLabel,
  WEATHER_ATTRIBUTION,
} from '@aurarisk/shared';
import { useEffect, useState } from 'react';
import { ActivityIndicator, Linking, StyleSheet, Text, View } from 'react-native';
import { useT } from '../i18n';
import { fetchNearbyReports, fetchRiskAssessment, RiskFetchResult } from '../services/api';

const MAX_REPORTS_SHOWN = 5;

interface Props {
  location: Location;
  /** Bump to refetch, e.g. after submitting a report. */
  refreshKey?: number;
}

/** Flood risk and recent community reports for the selected location (GET /api/risk, /api/reports). */
export default function RiskPanel({ location, refreshKey = 0 }: Props) {
  const [risk, setRisk] = useState<RiskFetchResult | null>(null);
  const [reports, setReports] = useState<CommunityReport[]>([]);
  const [loading, setLoading] = useState(true);
  const { lang, t } = useT();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    Promise.all([fetchRiskAssessment(location.lat, location.lon), fetchNearbyReports(location.lat, location.lon)]).then(
      ([riskResult, reportResult]) => {
        if (cancelled) return;
        setRisk(riskResult);
        setReports(reportResult.reports);
        setLoading(false);
      }
    );
    return () => {
      cancelled = true;
    };
  }, [location.lat, location.lon, refreshKey, lang]); // lang: refetch so the summary and tips follow it

  if (loading) return <ActivityIndicator accessibilityLabel={t('mobile.loadingRisk')} />;

  const data = risk?.data;
  const freshness = data ? describeFreshness(data, Date.now(), lang) : null;
  return (
    <View style={styles.section}>
      <Text style={styles.heading}>{t('mobile.floodRisk')}</Text>
      {risk?.isOffline && (
        <Text style={styles.banner}>
          {data ? t(risk.isStale ? 'mobile.offlineSavedOld' : 'mobile.offlineSaved') : t('mobile.offlineNoData')}
        </Text>
      )}
      {data ? (
        <View style={[styles.card, { borderLeftColor: RISK_LEVEL_COLORS[data.level] }]}>
          <View style={styles.levelRow}>
            <Text style={[styles.level, { color: RISK_LEVEL_COLORS[data.level] }]}>{riskLevelLabel(lang, data.level)}</Text>
            <Text style={styles.meta}>{t('mobile.score', { score: data.score })}</Text>
          </View>
          <Text style={styles.body}>{data.summary}</Text>
          {data.tips.map((tip) => (
            <Text key={tip} style={styles.body}>
              • {tip}
            </Text>
          ))}
          {freshness && (
            <Text style={freshness.outdated ? styles.outdated : styles.meta}>
              {freshness.label}
              {freshness.outdated ? `. ${t('freshness.mayHaveChanged')}` : ''}
            </Text>
          )}
          <Text
            style={styles.meta}
            accessibilityRole="link"
            onPress={() => Linking.openURL(WEATHER_ATTRIBUTION.url)}
          >
            {t('weather.attribution')} ({WEATHER_ATTRIBUTION.licence})
          </Text>
        </View>
      ) : (
        !risk?.isOffline && <Text style={styles.error}>{t('mobile.riskFailed')}</Text>
      )}

      <Text style={styles.notice} accessibilityRole="text">
        {t('notice.decisionSupport')}
      </Text>

      <Text style={styles.subheading}>{t('mobile.nearbyReports', { count: reports.length })}</Text>
      {reports.length === 0 && <Text style={styles.meta}>{t('mobile.noNearby')}</Text>}
      {reports.slice(0, MAX_REPORTS_SHOWN).map((r) => (
        <View key={r.id} style={styles.report}>
          <Text style={styles.reportTitle}>
            {categoryLabel(lang, r.category)} · {new Date(r.timestamp).toLocaleString(lang)}
          </Text>
          <Text style={styles.body} numberOfLines={3}>
            {r.note}
          </Text>
          <Text style={styles.meta}>
            {[r.ward, r.county].filter(Boolean).join(', ')}
            {r.ward || r.county ? ' · ' : ''}
            {reportStatusLabel(r, lang)}
          </Text>
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { gap: 10 },
  heading: { fontSize: 20, fontWeight: '600' },
  subheading: { fontSize: 16, fontWeight: '600', marginTop: 8 },
  banner: { backgroundColor: '#fff4ce', padding: 10, borderRadius: 6 },
  card: { borderWidth: 1, borderColor: '#ddd', borderLeftWidth: 6, borderRadius: 8, padding: 12, gap: 6 },
  levelRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' },
  level: { fontSize: 22, fontWeight: '700' },
  body: { fontSize: 14, lineHeight: 20 },
  meta: { fontSize: 13, color: '#555' },
  error: { color: '#b00020' },
  outdated: { fontSize: 13, color: '#7a4a00' },
  notice: { fontSize: 12, lineHeight: 17, color: '#444', backgroundColor: '#fff8e1', padding: 8, borderRadius: 6 },
  report: { paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#ddd', gap: 2 },
  reportTitle: { fontSize: 13, fontWeight: '600' },
});
