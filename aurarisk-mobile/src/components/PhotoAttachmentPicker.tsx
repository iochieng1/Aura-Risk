import { useState } from 'react';
import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from 'react-native';
import { deleteLocalPhoto, MAX_PHOTOS_PER_REPORT, pickPhotosFromLibrary, takePhoto } from '../photos/photoStore';

interface Props {
  photos: string[];
  onChange: (photos: string[]) => void;
}

/** Lets the user attach up to four photos. URIs point at prepared JPEGs in the document directory. */
export default function PhotoAttachmentPicker({ photos, onChange }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const remaining = MAX_PHOTOS_PER_REPORT - photos.length;

  const add = async (source: 'library' | 'camera') => {
    setError(null);
    setBusy(true);
    try {
      const added = source === 'library' ? await pickPhotosFromLibrary(remaining) : await takePhoto();
      if (added === 'permission_denied') {
        setError('Camera access was not granted.');
        return;
      }
      onChange([...photos, ...added].slice(0, MAX_PHOTOS_PER_REPORT));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not add photo.');
    } finally {
      setBusy(false);
    }
  };

  const remove = (uri: string) => {
    deleteLocalPhoto(uri);
    onChange(photos.filter((p) => p !== uri));
  };

  return (
    <View style={styles.container}>
      <Text style={styles.label}>
        Photos ({photos.length}/{MAX_PHOTOS_PER_REPORT})
      </Text>
      <View style={styles.grid}>
        {photos.map((uri, index) => (
          <View key={uri} style={styles.thumbWrap}>
            <Image source={{ uri }} style={styles.thumb} accessibilityLabel={`Attached photo ${index + 1}`} />
            <Pressable style={styles.removeBadge} onPress={() => remove(uri)} accessibilityRole="button" accessibilityLabel={`Remove photo ${index + 1}`}>
              <Text style={styles.removeText}>×</Text>
            </Pressable>
          </View>
        ))}
        {busy && <ActivityIndicator style={styles.thumb} />}
      </View>
      {remaining > 0 && !busy && (
        <View style={styles.actions}>
          <Pressable style={styles.button} onPress={() => add('library')} accessibilityRole="button">
            <Text>Choose photo</Text>
          </Pressable>
          <Pressable style={styles.button} onPress={() => add('camera')} accessibilityRole="button">
            <Text>Take photo</Text>
          </Pressable>
        </View>
      )}
      <Text style={styles.hint}>Location data embedded in photos is removed before upload.</Text>
      {error && <Text style={styles.error}>{error}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: 8 },
  label: { fontSize: 16 },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  thumbWrap: { position: 'relative' },
  thumb: { width: 72, height: 72, borderRadius: 6, backgroundColor: '#eee' },
  removeBadge: {
    position: 'absolute',
    top: -6,
    right: -6,
    width: 24,
    height: 24,
    borderRadius: 12,
    backgroundColor: '#000a',
    alignItems: 'center',
    justifyContent: 'center',
  },
  removeText: { color: '#fff', fontSize: 16, lineHeight: 18 },
  actions: { flexDirection: 'row', gap: 8 },
  button: { paddingVertical: 8, paddingHorizontal: 12, borderRadius: 6, borderWidth: 1, borderColor: '#ccc' },
  hint: { fontSize: 12, color: '#555' },
  error: { color: '#b00020' },
});
