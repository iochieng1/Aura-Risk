import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Linking, Pressable, StyleSheet, Switch, Text, TextInput, View } from 'react-native';
import { disablePushNotifications, enablePushNotifications, getStoredConsent } from '../notifications/push';
import { deviceTimeZone, isValidTime, isWithinQuietHours } from '../notifications/quietHours';
import {
  createSubscription,
  deleteSubscription,
  getCachedSubscriptions,
  listSubscriptions,
  MAX_SUBSCRIPTIONS,
} from '../notifications/subscriptions';
import { RiskLevel, Subscription } from '../types/api';

const LEVELS: RiskLevel[] = ['Advisory', 'Alert', 'Emergency'];

type ConsentState = 'loading' | 'off' | 'explaining' | 'requesting' | 'on' | 'denied';

export default function NotificationSettings() {
  const [consent, setConsent] = useState<ConsentState>('loading');
  const [message, setMessage] = useState<string | null>(null);
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);

  const [name, setName] = useState('');
  const [lat, setLat] = useState('');
  const [lon, setLon] = useState('');
  const [minLevel, setMinLevel] = useState<RiskLevel>('Alert');
  const [quietEnabled, setQuietEnabled] = useState(true);
  const [quietStart, setQuietStart] = useState('22:00');
  const [quietEnd, setQuietEnd] = useState('07:00');
  const [allowEmergency, setAllowEmergency] = useState(true);
  const [saving, setSaving] = useState(false);

  const loadSubscriptions = useCallback(async () => {
    setSubscriptions(await getCachedSubscriptions());
    try {
      setSubscriptions(await listSubscriptions());
    } catch {
      // Offline: the cached list is already shown.
    }
  }, []);

  useEffect(() => {
    getStoredConsent().then((stored) => setConsent(stored ? 'on' : 'off'));
    loadSubscriptions();
  }, [loadSubscriptions]);

  const onToggle = async (value: boolean) => {
    setMessage(null);
    if (value) {
      // Explain first; the OS prompt is only shown after the user agrees here.
      setConsent('explaining');
      return;
    }
    if (consent !== 'on') {
      setConsent('off');
      return;
    }
    try {
      await disablePushNotifications();
      setConsent('off');
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Could not turn alerts off. Try again when online.');
    }
  };

  const onAgree = async () => {
    setConsent('requesting');
    const result = await enablePushNotifications();
    if (result.status === 'enabled') {
      setConsent('on');
    } else if (result.status === 'denied') {
      setConsent('denied');
    } else {
      setConsent('off');
      setMessage(result.reason);
    }
  };

  const onAdd = async () => {
    setMessage(null);
    const latNum = Number(lat);
    const lonNum = Number(lon);
    if (!name.trim() || name.length > 100) return setMessage('Enter a name (up to 100 characters).');
    if (!lat.trim() || !Number.isFinite(latNum) || latNum < -90 || latNum > 90) return setMessage('Latitude must be between -90 and 90.');
    if (!lon.trim() || !Number.isFinite(lonNum) || lonNum < -180 || lonNum > 180) return setMessage('Longitude must be between -180 and 180.');
    if (quietEnabled && (!isValidTime(quietStart) || !isValidTime(quietEnd))) return setMessage('Quiet hours must use HH:MM (24-hour).');

    setSaving(true);
    try {
      await createSubscription({
        name: name.trim(),
        lat: latNum,
        lon: lonNum,
        min_level: minLevel,
        quiet_hours: quietEnabled ? { start: quietStart, end: quietEnd, timezone: deviceTimeZone() } : null,
        allow_emergency_during_quiet_hours: quietEnabled && allowEmergency,
      });
      setName('');
      setLat('');
      setLon('');
      setSubscriptions(await getCachedSubscriptions());
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Could not save location.');
    } finally {
      setSaving(false);
    }
  };

  const onRemove = async (id: string) => {
    try {
      await deleteSubscription(id);
      setSubscriptions(await getCachedSubscriptions());
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Could not remove location.');
    }
  };

  if (consent === 'loading') return <ActivityIndicator />;

  return (
    <View style={styles.section}>
      <Text style={styles.heading}>Flood alerts</Text>

      <View style={styles.row}>
        <Text style={styles.label}>Push notifications</Text>
        <Switch
          value={consent === 'on' || consent === 'explaining' || consent === 'requesting'}
          onValueChange={onToggle}
          disabled={consent === 'requesting'}
          accessibilityLabel="Push notifications for flood alerts"
        />
      </View>

      {(consent === 'explaining' || consent === 'requesting') && (
        <View style={styles.card}>
          <Text style={styles.body}>
            AuraRisk can notify you when flood risk rises at the locations you follow below. We send your
            device&apos;s notification token to our server and use it only for these alerts. Nothing is sent
            during your quiet hours unless you allow emergency alerts. You can turn this off at any time.
          </Text>
          <View style={styles.row}>
            <Pressable style={styles.button} onPress={onAgree} disabled={consent === 'requesting'} accessibilityRole="button">
              <Text style={styles.buttonText}>{consent === 'requesting' ? 'Requesting…' : 'Allow alerts'}</Text>
            </Pressable>
            <Pressable style={styles.secondaryButton} onPress={() => setConsent('off')} accessibilityRole="button">
              <Text>Not now</Text>
            </Pressable>
          </View>
        </View>
      )}

      {consent === 'denied' && (
        <View style={styles.card}>
          <Text style={styles.body}>Notifications are blocked in system settings for this app.</Text>
          <Pressable style={styles.secondaryButton} onPress={() => Linking.openSettings()} accessibilityRole="button">
            <Text>Open settings</Text>
          </Pressable>
        </View>
      )}

      {message && <Text style={styles.error}>{message}</Text>}

      <Text style={styles.subheading}>Locations you follow ({subscriptions.length}/{MAX_SUBSCRIPTIONS})</Text>
      {subscriptions.map((s) => (
        <View key={s.id} style={styles.subscription}>
          <View style={{ flex: 1 }}>
            <Text style={styles.label}>{s.name}</Text>
            <Text style={styles.meta}>
              {s.min_level}+ · {s.lat.toFixed(3)}, {s.lon.toFixed(3)}
              {s.quiet_hours ? ` · quiet ${s.quiet_hours.start}–${s.quiet_hours.end}` : ''}
              {isWithinQuietHours(s.quiet_hours) ? ' (quiet now)' : ''}
            </Text>
          </View>
          <Pressable onPress={() => onRemove(s.id)} accessibilityRole="button" accessibilityLabel={`Stop following ${s.name}`}>
            <Text style={styles.remove}>Remove</Text>
          </Pressable>
        </View>
      ))}

      {subscriptions.length < MAX_SUBSCRIPTIONS && (
        <View style={styles.card}>
          <TextInput style={styles.input} placeholder="Name (e.g. Home)" value={name} onChangeText={setName} maxLength={100} />
          <View style={styles.row}>
            <TextInput style={[styles.input, styles.half]} placeholder="Latitude" value={lat} onChangeText={setLat} keyboardType="numbers-and-punctuation" />
            <TextInput style={[styles.input, styles.half]} placeholder="Longitude" value={lon} onChangeText={setLon} keyboardType="numbers-and-punctuation" />
          </View>
          <Text style={styles.meta}>Notify me at</Text>
          <View style={styles.row}>
            {LEVELS.map((level) => (
              <Pressable
                key={level}
                onPress={() => setMinLevel(level)}
                style={[styles.chip, minLevel === level && styles.chipSelected]}
                accessibilityRole="radio"
                accessibilityState={{ selected: minLevel === level }}
              >
                <Text style={minLevel === level ? styles.chipTextSelected : undefined}>{level}+</Text>
              </Pressable>
            ))}
          </View>
          <View style={styles.row}>
            <Text style={styles.label}>Quiet hours</Text>
            <Switch value={quietEnabled} onValueChange={setQuietEnabled} accessibilityLabel="Quiet hours" />
          </View>
          {quietEnabled && (
            <>
              <View style={styles.row}>
                <TextInput style={[styles.input, styles.half]} value={quietStart} onChangeText={setQuietStart} placeholder="22:00" accessibilityLabel="Quiet hours start" />
                <TextInput style={[styles.input, styles.half]} value={quietEnd} onChangeText={setQuietEnd} placeholder="07:00" accessibilityLabel="Quiet hours end" />
              </View>
              <View style={styles.row}>
                <Text style={[styles.body, { flex: 1 }]}>Allow Emergency alerts during quiet hours</Text>
                <Switch value={allowEmergency} onValueChange={setAllowEmergency} accessibilityLabel="Allow emergency alerts during quiet hours" />
              </View>
              <Text style={styles.meta}>Time zone: {deviceTimeZone()}</Text>
            </>
          )}
          <Pressable style={styles.button} onPress={onAdd} disabled={saving} accessibilityRole="button">
            <Text style={styles.buttonText}>{saving ? 'Saving…' : 'Follow location'}</Text>
          </Pressable>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { gap: 12 },
  heading: { fontSize: 20, fontWeight: '600' },
  subheading: { fontSize: 16, fontWeight: '600', marginTop: 8 },
  row: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 8 },
  label: { fontSize: 16 },
  body: { fontSize: 14, lineHeight: 20 },
  meta: { fontSize: 13, color: '#555' },
  error: { color: '#b00020' },
  card: { borderWidth: 1, borderColor: '#ddd', borderRadius: 8, padding: 12, gap: 10 },
  input: { borderWidth: 1, borderColor: '#ccc', borderRadius: 6, paddingHorizontal: 10, paddingVertical: 8 },
  half: { flex: 1 },
  button: { backgroundColor: '#0b5cad', paddingVertical: 10, paddingHorizontal: 14, borderRadius: 6, alignItems: 'center' },
  buttonText: { color: '#fff', fontWeight: '600' },
  secondaryButton: { paddingVertical: 10, paddingHorizontal: 14, borderRadius: 6, borderWidth: 1, borderColor: '#ccc', alignItems: 'center' },
  chip: { flex: 1, alignItems: 'center', paddingVertical: 8, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  chipSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  chipTextSelected: { color: '#fff' },
  subscription: { flexDirection: 'row', alignItems: 'center', paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#ddd' },
  remove: { color: '#b00020' },
});
