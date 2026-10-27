import { KENYA_COUNTIES_SORTED, type KenyaCounty, type Location, MAX_WARD_BYTES, REPORT_CATEGORIES } from '@aurarisk/shared';
import { useState } from 'react';
import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { ReportRejectedError, ReportValidationError, submitReport } from '../services/api';
import { ReportCategory } from '../types/api';
import PhotoAttachmentPicker from './PhotoAttachmentPicker';

interface Props {
  /** Chosen with LocationPicker; null until the user picks one. */
  location: Location | null;
  onSubmitted?: () => void;
}

export default function ReportComposer({ location, onSubmitted }: Props) {
  const [category, setCategory] = useState<ReportCategory>('flooding');
  const [note, setNote] = useState('');
  // County and ward stay between reports: people usually report nearby.
  const [county, setCounty] = useState<KenyaCounty | null>(null);
  const [countyQuery, setCountyQuery] = useState('');
  const [ward, setWard] = useState('');
  const countyMatches = countyQuery.trim()
    ? KENYA_COUNTIES_SORTED.filter((c) => c.toLowerCase().startsWith(countyQuery.trim().toLowerCase())).slice(0, 6)
    : [];
  const [photos, setPhotos] = useState<string[]>([]);
  const [status, setStatus] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async () => {
    if (!location) return setStatus('Choose a location above first.');

    setSubmitting(true);
    setStatus(null);
    try {
      const area = county ? { county, ...(ward.trim() && { ward: ward.trim() }) } : {};
      const result = await submitReport({ location, category, note, ...area }, photos);
      setStatus(result.queued ? 'Saved. It will be sent when you are back online.' : 'Report sent. Photos appear after a safety check.');
      setNote('');
      setPhotos([]);
      onSubmitted?.();
    } catch (err) {
      if (err instanceof ReportValidationError) setStatus(err.message);
      else setStatus(err instanceof ReportRejectedError ? 'The server rejected this report. Check the details.' : 'Could not send report.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <View style={styles.section}>
      <Text style={styles.heading}>Report a flood issue</Text>
      <Text style={styles.meta} numberOfLines={2}>
        {location ? `At ${location.name}` : 'Choose a location above to report.'}
      </Text>
      <View style={styles.chips}>
        {REPORT_CATEGORIES.map((c) => (
          <Pressable
            key={c.value}
            onPress={() => setCategory(c.value)}
            style={[styles.chip, category === c.value && styles.chipSelected]}
            accessibilityRole="radio"
            accessibilityState={{ selected: category === c.value }}
          >
            <Text style={category === c.value ? styles.chipTextSelected : undefined}>{c.label}</Text>
          </Pressable>
        ))}
      </View>
      <TextInput style={[styles.input, styles.note]} placeholder="What do you see?" value={note} onChangeText={setNote} multiline />
      {county ? (
        <View style={styles.countyRow}>
          <Text>County: {county}</Text>
          <Pressable onPress={() => { setCounty(null); setWard(''); }} accessibilityRole="button" accessibilityLabel={`Change county from ${county}`}>
            <Text style={styles.link}>Change</Text>
          </Pressable>
        </View>
      ) : (
        <>
          <TextInput
            style={styles.input}
            placeholder="County (optional), e.g. Kisumu"
            value={countyQuery}
            onChangeText={setCountyQuery}
            autoCorrect={false}
          />
          {countyMatches.length > 0 && (
            <View style={styles.chips}>
              {countyMatches.map((c) => (
                <Pressable key={c} onPress={() => { setCounty(c); setCountyQuery(''); }} style={styles.chip} accessibilityRole="button">
                  <Text>{c}</Text>
                </Pressable>
              ))}
            </View>
          )}
        </>
      )}
      {county && (
        <TextInput style={styles.input} placeholder="Ward (optional), e.g. Nyalenda A" value={ward} onChangeText={setWard} maxLength={MAX_WARD_BYTES} />
      )}
      <PhotoAttachmentPicker photos={photos} onChange={setPhotos} />
      <Pressable
        style={[styles.button, (!location || submitting) && styles.disabled]}
        onPress={onSubmit}
        disabled={submitting || !location}
        accessibilityRole="button"
      >
        <Text style={styles.buttonText}>{submitting ? 'Sending…' : 'Submit report'}</Text>
      </Pressable>
      {status && <Text accessibilityLiveRegion="polite">{status}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { gap: 10 },
  heading: { fontSize: 20, fontWeight: '600' },
  meta: { fontSize: 13, color: '#555' },
  input: { borderWidth: 1, borderColor: '#ccc', borderRadius: 6, paddingHorizontal: 10, paddingVertical: 8 },
  note: { minHeight: 72, textAlignVertical: 'top' },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: { paddingVertical: 6, paddingHorizontal: 12, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  chipSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  chipTextSelected: { color: '#fff' },
  button: { backgroundColor: '#0b5cad', paddingVertical: 10, borderRadius: 6, alignItems: 'center' },
  buttonText: { color: '#fff', fontWeight: '600' },
  disabled: { opacity: 0.6 },
  countyRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  link: { color: '#0b5cad', fontWeight: '600' },
});
