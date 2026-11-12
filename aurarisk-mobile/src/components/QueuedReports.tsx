import { categoryLabel, REPORT_CATEGORIES } from '@aurarisk/shared';
import { useState } from 'react';
import { Alert, Platform, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { useT } from '../i18n';
import { ReportValidationError } from '../services/api';
import { QueuedReport } from '../storage/reportQueue';
import { discardReport, editReport, retryReport } from '../sync/queueActions';
import { ReportCategory } from '../types/api';

interface Props {
  /** The whole report queue, from useOfflineSync. */
  queue: QueuedReport[];
  /** Called after the user makes a report due again, so it can be sent. */
  onRequeued: () => void;
}

/** Lists reports that haven't been sent, with the reason, and lets the user retry, edit or discard them. */
export default function QueuedReports({ queue, onRequeued }: Props) {
  const { t } = useT();
  if (queue.length === 0) return null;

  return (
    <View style={styles.section}>
      <Text style={styles.heading}>{t('mobile.queueHeading')}</Text>
      {queue.map((item) => (
        <QueuedReportRow key={item.tempId} item={item} onRequeued={onRequeued} />
      ))}
    </View>
  );
}

function QueuedReportRow({ item, onRequeued }: { item: QueuedReport; onRequeued: () => void }) {
  const { lang, t } = useT();
  const [editing, setEditing] = useState(false);
  const [category, setCategory] = useState<ReportCategory>(item.category);
  const [note, setNote] = useState(item.note);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const failed = item.status === 'failed';

  const run = async (action: () => Promise<void>, requeue: boolean) => {
    setBusy(true);
    setError(null);
    try {
      await action();
      if (requeue) onRequeued();
    } catch (err) {
      setError(err instanceof ReportValidationError ? err.message : t('mobile.queueActionFailed'));
    } finally {
      setBusy(false);
    }
  };

  const discard = () => run(() => discardReport(item.tempId), false);
  const confirmDiscard = () => {
    // Alert.alert does nothing on web.
    if (Platform.OS === 'web') {
      if (globalThis.confirm?.(`${t('mobile.discardTitle')}\n${t('mobile.discardBody')}`)) discard();
      return;
    }
    Alert.alert(t('mobile.discardTitle'), t('mobile.discardBody'), [
      { text: t('mobile.cancel'), style: 'cancel' },
      { text: t('mobile.discard'), style: 'destructive', onPress: discard },
    ]);
  };

  const save = () =>
    run(async () => {
      await editReport(item.tempId, { category, note });
      setEditing(false);
    }, true);

  let statusText: string;
  if (failed) {
    statusText = item.lastError ? t('mobile.queueRejected', { reason: item.lastError }) : t('mobile.queueRejectedNoReason');
  } else if (item.serverReportId) {
    statusText = t('mobile.photosWaiting', { count: item.photos.length });
  } else {
    statusText = t('mobile.queueWaiting');
  }

  return (
    <View style={[styles.card, failed && styles.cardFailed]}>
      <Text style={styles.title} numberOfLines={1}>
        {categoryLabel(lang, item.category)} · {item.location.name}
      </Text>
      {!editing && <Text numberOfLines={2}>{item.note}</Text>}
      <Text style={failed ? styles.failedText : styles.meta} accessibilityLiveRegion="polite">
        {statusText}
      </Text>
      {!failed && item.lastError && <Text style={styles.meta}>{t('mobile.queueLastError', { reason: item.lastError })}</Text>}

      {editing ? (
        <>
          <View style={styles.chips}>
            {REPORT_CATEGORIES.map((c) => (
              <Pressable
                key={c.value}
                onPress={() => setCategory(c.value)}
                style={[styles.chip, category === c.value && styles.chipSelected]}
                accessibilityRole="radio"
                accessibilityState={{ selected: category === c.value }}
              >
                <Text style={category === c.value ? styles.chipTextSelected : undefined}>{categoryLabel(lang, c.value)}</Text>
              </Pressable>
            ))}
          </View>
          <TextInput style={styles.input} value={note} onChangeText={setNote} multiline placeholder={t('mobile.notePlaceholder')} />
          <View style={styles.actions}>
            <Pressable style={[styles.button, styles.primary, busy && styles.disabled]} onPress={save} disabled={busy} accessibilityRole="button">
              <Text style={styles.primaryText}>{t('mobile.saveAndSend')}</Text>
            </Pressable>
            <Pressable
              style={styles.button}
              onPress={() => {
                setEditing(false);
                setCategory(item.category);
                setNote(item.note);
              }}
              accessibilityRole="button"
            >
              <Text>{t('mobile.cancel')}</Text>
            </Pressable>
          </View>
        </>
      ) : (
        <View style={styles.actions}>
          <Pressable
            style={[styles.button, busy && styles.disabled]}
            onPress={() => run(() => retryReport(item.tempId), true)}
            disabled={busy}
            accessibilityRole="button"
          >
            <Text>{failed ? t('mobile.retry') : t('mobile.sendNow')}</Text>
          </Pressable>
          {/* Only a report the server never accepted can be changed. */}
          {failed && !item.serverReportId && (
            <Pressable style={[styles.button, busy && styles.disabled]} onPress={() => setEditing(true)} disabled={busy} accessibilityRole="button">
              <Text>{t('mobile.edit')}</Text>
            </Pressable>
          )}
          <Pressable style={[styles.button, busy && styles.disabled]} onPress={confirmDiscard} disabled={busy} accessibilityRole="button">
            <Text style={styles.failedText}>{t('mobile.discard')}</Text>
          </Pressable>
        </View>
      )}
      {error && <Text style={styles.failedText}>{error}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { gap: 10 },
  heading: { fontSize: 20, fontWeight: '600' },
  card: { borderWidth: 1, borderColor: '#ccc', borderRadius: 6, padding: 10, gap: 6 },
  cardFailed: { borderColor: '#b00020' },
  title: { fontWeight: '600' },
  meta: { fontSize: 13, color: '#555' },
  failedText: { color: '#b00020' },
  input: { borderWidth: 1, borderColor: '#ccc', borderRadius: 6, paddingHorizontal: 10, paddingVertical: 8, minHeight: 72, textAlignVertical: 'top' },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: { paddingVertical: 6, paddingHorizontal: 12, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  chipSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  chipTextSelected: { color: '#fff' },
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  button: { paddingVertical: 8, paddingHorizontal: 12, borderRadius: 6, borderWidth: 1, borderColor: '#ccc' },
  primary: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  primaryText: { color: '#fff', fontWeight: '600' },
  disabled: { opacity: 0.6 },
});
