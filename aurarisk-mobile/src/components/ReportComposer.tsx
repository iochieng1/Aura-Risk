import { useState } from 'react';
import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { ReportRejectedError, submitReport } from '../services/api';
import { ReportCategory } from '../types/api';
import PhotoAttachmentPicker from './PhotoAttachmentPicker';

const CATEGORIES: { value: ReportCategory; label: string }[] = [
  { value: 'flooding', label: 'Flooding' },
  { value: 'road_blocked', label: 'Road blocked' },
  { value: 'water_rising', label: 'Water rising' },
  { value: 'drainage_issue', label: 'Drainage issue' },
];

interface Props {
  onSubmitted?: () => void;
}

export default function ReportComposer({ onSubmitted }: Props) {
  const [locationName, setLocationName] = useState('');
  const [lat, setLat] = useState('');
  const [lon, setLon] = useState('');
  const [category, setCategory] = useState<ReportCategory>('flooding');
  const [note, setNote] = useState('');
  const [photos, setPhotos] = useState<string[]>([]);
  const [status, setStatus] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async () => {
    const latNum = Number(lat);
    const lonNum = Number(lon);
    if (!locationName.trim() || !note.trim()) return setStatus('Location name and note are required.');
    if (!lat.trim() || !lon.trim() || !Number.isFinite(latNum) || !Number.isFinite(lonNum) || Math.abs(latNum) > 90 || Math.abs(lonNum) > 180) {
      return setStatus('Enter a valid latitude and longitude.');
    }

    setSubmitting(true);
    setStatus(null);
    try {
      const result = await submitReport(
        { location: { name: locationName.trim(), lat: latNum, lon: lonNum }, category, note: note.trim() },
        photos
      );
      setStatus(result.queued ? 'Saved. It will be sent when you are back online.' : 'Report sent. Photos appear after a safety check.');
      setNote('');
      setPhotos([]);
      onSubmitted?.();
    } catch (err) {
      setStatus(err instanceof ReportRejectedError ? 'The server rejected this report. Check the details.' : 'Could not send report.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <View style={styles.section}>
      <Text style={styles.heading}>Report a flood issue</Text>
      <TextInput style={styles.input} placeholder="Location name" value={locationName} onChangeText={setLocationName} />
      <View style={styles.row}>
        <TextInput style={[styles.input, styles.half]} placeholder="Latitude" value={lat} onChangeText={setLat} keyboardType="numbers-and-punctuation" />
        <TextInput style={[styles.input, styles.half]} placeholder="Longitude" value={lon} onChangeText={setLon} keyboardType="numbers-and-punctuation" />
      </View>
      <View style={styles.chips}>
        {CATEGORIES.map((c) => (
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
      <PhotoAttachmentPicker photos={photos} onChange={setPhotos} />
      <Pressable style={styles.button} onPress={onSubmit} disabled={submitting} accessibilityRole="button">
        <Text style={styles.buttonText}>{submitting ? 'Sending…' : 'Submit report'}</Text>
      </Pressable>
      {status && <Text accessibilityLiveRegion="polite">{status}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { gap: 10 },
  heading: { fontSize: 20, fontWeight: '600' },
  row: { flexDirection: 'row', gap: 8 },
  input: { borderWidth: 1, borderColor: '#ccc', borderRadius: 6, paddingHorizontal: 10, paddingVertical: 8 },
  half: { flex: 1 },
  note: { minHeight: 72, textAlignVertical: 'top' },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: { paddingVertical: 6, paddingHorizontal: 12, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  chipSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  chipTextSelected: { color: '#fff' },
  button: { backgroundColor: '#0b5cad', paddingVertical: 10, borderRadius: 6, alignItems: 'center' },
  buttonText: { color: '#fff', fontWeight: '600' },
});
