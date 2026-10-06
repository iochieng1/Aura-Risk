import { type FieldErrors, type Location, parseCoordinates, searchPlaces, validateLocation } from '@aurarisk/shared';
import Constants from 'expo-constants';
import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Linking, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { formatCoordinates, getDeviceLocation, getLocationPermission, LocationPermission } from '../location/deviceLocation';

const SEARCH_DEBOUNCE_MS = 400;
const USER_AGENT = `AuraRisk-mobile/${Constants.expoConfig?.version ?? 'dev'}`;

type ManualMode = 'search' | 'coordinates';

type Notice = { text: string; openSettings?: boolean } | null;

interface Props {
  value: Location | null;
  onChange: (location: Location) => void;
}

/**
 * Picks a location from the device (when the user allows it) or manually by place search or typed
 * coordinates. Manual entry is always available, so a denied permission never blocks the app.
 */
export default function LocationPicker({ value, onChange }: Props) {
  const [permission, setPermission] = useState<LocationPermission | null>(null);
  const [locating, setLocating] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);
  const [mode, setMode] = useState<ManualMode>('search');

  const [query, setQuery] = useState('');
  const [results, setResults] = useState<Location[]>([]);
  const [searching, setSearching] = useState(false);
  const searchTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const searchSeq = useRef(0);

  const [manualName, setManualName] = useState('');
  const [lat, setLat] = useState('');
  const [lon, setLon] = useState('');
  const [errors, setErrors] = useState<FieldErrors>({});

  useEffect(() => {
    getLocationPermission().then(setPermission);
    return () => clearTimeout(searchTimer.current);
  }, []);

  const useMyLocation = async () => {
    setLocating(true);
    setNotice(null);
    const result = await getDeviceLocation();
    setLocating(false);

    switch (result.status) {
      case 'ok':
        setPermission('granted');
        onChange(result.location);
        return;
      case 'denied':
        setPermission(result.blocked ? 'blocked' : 'denied');
        setNotice(
          result.blocked
            ? { text: 'Location access is off for AuraRisk. Turn it on in Settings, or choose a place below.', openSettings: true }
            : { text: 'No problem — search for a place or enter coordinates below.' }
        );
        return;
      case 'services_disabled':
        setNotice({ text: 'Location services are turned off on this device. Turn them on, or choose a place below.', openSettings: true });
        return;
      case 'unavailable':
        setNotice({ text: 'Could not get your position right now. Try again, or choose a place below.' });
        return;
    }
  };

  const onQueryChange = (text: string) => {
    setQuery(text);
    clearTimeout(searchTimer.current);
    const seq = ++searchSeq.current;
    if (text.trim().length < 3) {
      setResults([]);
      setSearching(false);
      return;
    }
    setSearching(true);
    searchTimer.current = setTimeout(async () => {
      const found = await searchPlaces(text, { userAgent: USER_AGENT });
      // Ignore responses that arrive after the user has typed something newer.
      if (seq !== searchSeq.current) return;
      setResults(found);
      setSearching(false);
    }, SEARCH_DEBOUNCE_MS);
  };

  const pickResult = (location: Location) => {
    setResults([]);
    setQuery(location.name.split(',')[0]);
    setNotice(null);
    onChange(location);
  };

  const useCoordinates = () => {
    const coords = parseCoordinates(lat, lon);
    if (!coords.ok) return setErrors(coords.errors);
    const name = manualName.trim() || `Pinned location (${formatCoordinates(coords.value.lat, coords.value.lon)})`;
    const location = validateLocation({ name, ...coords.value });
    if (!location.ok) return setErrors(location.errors);
    setErrors({});
    setNotice(null);
    onChange(location.value);
  };

  return (
    <View style={styles.section}>
      <Text style={styles.heading}>Location</Text>

      <View style={styles.selected} accessibilityLiveRegion="polite">
        {value ? (
          <>
            <Text style={styles.selectedName} numberOfLines={2}>
              {value.name}
            </Text>
            <Text style={styles.meta}>{formatCoordinates(value.lat, value.lon)}</Text>
          </>
        ) : (
          <Text style={styles.meta}>No location selected yet.</Text>
        )}
      </View>

      <Pressable
        style={[styles.button, locating && styles.disabled]}
        onPress={useMyLocation}
        disabled={locating}
        accessibilityRole="button"
        accessibilityHint="Asks for location permission if needed"
      >
        {locating ? <ActivityIndicator color="#fff" /> : <Text style={styles.buttonText}>Use my location</Text>}
      </Pressable>
      {permission === 'blocked' && !notice && (
        <Text style={styles.meta}>Location access is off. You can still search or enter coordinates.</Text>
      )}

      {notice && (
        <View style={styles.notice}>
          <Text style={styles.body}>{notice.text}</Text>
          {notice.openSettings && (
            <Pressable style={styles.secondaryButton} onPress={() => Linking.openSettings()} accessibilityRole="button">
              <Text>Open settings</Text>
            </Pressable>
          )}
        </View>
      )}

      <View style={styles.tabs} accessibilityRole="tablist">
        {(['search', 'coordinates'] as const).map((m) => (
          <Pressable
            key={m}
            onPress={() => setMode(m)}
            style={[styles.tab, mode === m && styles.tabSelected]}
            accessibilityRole="tab"
            accessibilityState={{ selected: mode === m }}
          >
            <Text style={mode === m ? styles.tabTextSelected : undefined}>{m === 'search' ? 'Search a place' : 'Enter coordinates'}</Text>
          </Pressable>
        ))}
      </View>

      {mode === 'search' ? (
        <View style={styles.group}>
          <TextInput
            style={styles.input}
            placeholder="Town, street, or landmark"
            value={query}
            onChangeText={onQueryChange}
            autoCorrect={false}
            returnKeyType="search"
            accessibilityLabel="Search for a place"
          />
          {searching && <ActivityIndicator />}
          {!searching && query.trim().length >= 3 && results.length === 0 && (
            <Text style={styles.meta}>No matches. Try a nearby town, or enter coordinates.</Text>
          )}
          {results.map((r) => (
            <Pressable key={`${r.lat},${r.lon}`} style={styles.result} onPress={() => pickResult(r)} accessibilityRole="button">
              <Text numberOfLines={2}>{r.name}</Text>
            </Pressable>
          ))}
          {results.length > 0 && <Text style={styles.attribution}>Search © OpenStreetMap contributors</Text>}
        </View>
      ) : (
        <View style={styles.group}>
          <TextInput
            style={styles.input}
            placeholder="Name (optional)"
            value={manualName}
            onChangeText={setManualName}
            accessibilityLabel="Location name"
          />
          {errors.name && <Text style={styles.error}>{errors.name}</Text>}
          <View style={styles.row}>
            <TextInput
              style={[styles.input, styles.half]}
              placeholder="Latitude"
              value={lat}
              onChangeText={setLat}
              keyboardType="numbers-and-punctuation"
              accessibilityLabel="Latitude"
            />
            <TextInput
              style={[styles.input, styles.half]}
              placeholder="Longitude"
              value={lon}
              onChangeText={setLon}
              keyboardType="numbers-and-punctuation"
              accessibilityLabel="Longitude"
            />
          </View>
          {(errors.lat || errors.lon) && <Text style={styles.error}>{errors.lat ?? errors.lon}</Text>}
          <Pressable style={styles.secondaryButton} onPress={useCoordinates} accessibilityRole="button">
            <Text>Use these coordinates</Text>
          </Pressable>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { gap: 10 },
  heading: { fontSize: 20, fontWeight: '600' },
  group: { gap: 8 },
  row: { flexDirection: 'row', gap: 8 },
  selected: { borderWidth: 1, borderColor: '#ddd', borderRadius: 8, padding: 12, gap: 2 },
  selectedName: { fontSize: 16, fontWeight: '500' },
  body: { fontSize: 14, lineHeight: 20 },
  meta: { fontSize: 13, color: '#555' },
  error: { color: '#b00020' },
  notice: { backgroundColor: '#fff4ce', borderRadius: 6, padding: 10, gap: 8 },
  input: { borderWidth: 1, borderColor: '#ccc', borderRadius: 6, paddingHorizontal: 10, paddingVertical: 8 },
  half: { flex: 1 },
  button: { backgroundColor: '#0b5cad', paddingVertical: 10, borderRadius: 6, alignItems: 'center', minHeight: 40, justifyContent: 'center' },
  buttonText: { color: '#fff', fontWeight: '600' },
  disabled: { opacity: 0.6 },
  secondaryButton: { paddingVertical: 10, paddingHorizontal: 14, borderRadius: 6, borderWidth: 1, borderColor: '#ccc', alignItems: 'center' },
  tabs: { flexDirection: 'row', gap: 8 },
  tab: { flex: 1, alignItems: 'center', paddingVertical: 8, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  tabSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  tabTextSelected: { color: '#fff' },
  result: { paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#ddd' },
  attribution: { fontSize: 11, color: '#777' },
});
