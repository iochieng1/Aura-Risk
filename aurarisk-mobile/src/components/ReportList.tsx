import { categoryLabel, type CommunityReport, reportStatusLabel } from '@aurarisk/shared';
import { StyleSheet, Text, View } from 'react-native';
import { useT } from '../i18n';

/** One report's category, time, place, status and note. Shared by the list view and the map's detail card. */
export function ReportSummary({ report }: { report: CommunityReport }) {
  const { lang, t } = useT();
  const place = [report.ward, report.county].filter(Boolean).join(', ') || report.location.name;
  return (
    <View style={styles.summary}>
      <Text style={styles.title}>
        {categoryLabel(lang, report.category)} · {new Date(report.timestamp).toLocaleString(lang)}
      </Text>
      <Text style={styles.body}>{report.note}</Text>
      <Text style={styles.meta}>
        {place}
        {report.location_approximate ? ` (${t('report.approximate')})` : ''} · {reportStatusLabel(report, lang)}
      </Text>
    </View>
  );
}

/** The map's reports as a plain list: the accessible alternative to the map, and a view in its own right. */
export default function ReportList({ reports }: { reports: CommunityReport[] }) {
  const { t } = useT();
  if (reports.length === 0) return <Text style={styles.meta}>{t('mobile.noNearby')}</Text>;
  return (
    <View accessibilityRole="list">
      {reports.map((r) => (
        <View key={r.id} style={styles.item}>
          <ReportSummary report={r} />
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  item: { paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#ddd' },
  summary: { gap: 2 },
  title: { fontSize: 14, fontWeight: '600' },
  body: { fontSize: 14, lineHeight: 20 },
  meta: { fontSize: 13, color: '#555' },
});
