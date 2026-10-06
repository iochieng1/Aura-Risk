import { type CommunityReport, type Location, REPORT_CATEGORIES, RISK_LEVEL_COLORS } from '@aurarisk/shared';
import { useEffect, useState } from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';
import { fetchNearbyReports, fetchRiskAssessment, RiskFetchResult } from '../services/api';

const MAX_REPORTS_SHOWN = 5;

const categoryLabel = (value: string) => REPORT_CATEGORIES.find((c) => c.value === value)?.label ?? value;

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
  }, [location.lat, location.lon, refreshKey]);

  if (loading) return <ActivityIndicator accessibilityLabel="Loading flood risk" />;

  const data = risk?.data;
  return (
    <View style={styles.section}>
      <Text style={styles.heading}>Flood risk</Text>
      {risk?.isOffline && (
        <Text style={styles.banner}>
          {data ? `Offline — showing saved data${risk.isStale ? ' older than a day' : ''}.` : 'Offline and no saved data for this place.'}
        </Text>
      )}
      {data ? (
        <View style={[styles.card, { borderLeftColor: RISK_LEVEL_COLORS[data.level] }]}>
          <View style={styles.levelRow}>
            <Text style={[styles.level, { color: RISK_LEVEL_COLORS[data.level] }]}>{data.level}</Text>
            <Text style={styles.meta}>Score {data.score}/100</Text>
          </View>
          <Text style={styles.body}>{data.summary}</Text>
          {data.tips.map((tip) => (
            <Text key={tip} style={styles.body}>
              • {tip}
            </Text>
          ))}
        </View>
      ) : (
        !risk?.isOffline && <Text style={styles.error}>Could not load flood risk for this place.</Text>
      )}

      <Text style={styles.subheading}>Nearby reports ({reports.length})</Text>
      {reports.length === 0 && <Text style={styles.meta}>No community reports within 10 km.</Text>}
      {reports.slice(0, MAX_REPORTS_SHOWN).map((r) => (
        <View key={r.id} style={styles.report}>
          <Text style={styles.reportTitle}>
            {categoryLabel(r.category)} · {new Date(r.timestamp).toLocaleString()}
          </Text>
          <Text style={styles.body} numberOfLines={3}>
            {r.note}
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
  report: { paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#ddd', gap: 2 },
  reportTitle: { fontSize: 13, fontWeight: '600' },
});
