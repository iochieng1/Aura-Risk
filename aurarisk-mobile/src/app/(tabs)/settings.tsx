import { WEATHER_ATTRIBUTION } from '@aurarisk/shared';
import Constants from 'expo-constants';
import { useState } from 'react';
import { ActivityIndicator, Alert, Linking, Pressable, StyleSheet, Text, View } from 'react-native';
import { deleteAccount } from '../../auth/authClient';
import LanguageToggle from '../../components/LanguageToggle';
import Screen from '../../components/Screen';
import { useT } from '../../i18n';
import { clearLocalPushConsent } from '../../notifications/push';
import { clearCachedSubscriptions } from '../../notifications/subscriptions';

const OSM_COPYRIGHT_URL = 'https://www.openstreetmap.org/copyright';

type DeleteState = 'idle' | 'deleting' | 'deleted' | 'failed';

/** Settings tab: language, account deletion, and about/attribution. */
export default function SettingsScreen() {
  const { t } = useT();
  const [deleteState, setDeleteState] = useState<DeleteState>('idle');

  const runDelete = async () => {
    setDeleteState('deleting');
    try {
      await deleteAccount();
      // The server removed these with the account; drop the local copies so nothing re-creates them.
      await Promise.all([clearCachedSubscriptions(), clearLocalPushConsent()]).catch(() => undefined);
      setDeleteState('deleted');
    } catch (err) {
      console.warn('Account deletion failed:', err);
      setDeleteState('failed');
    }
  };

  const confirmDelete = () =>
    Alert.alert(t('mobile.deleteAccountTitle'), t('mobile.deleteAccountBody'), [
      { text: t('mobile.cancel'), style: 'cancel' },
      { text: t('mobile.deleteAccount'), style: 'destructive', onPress: runDelete },
    ]);

  return (
    <Screen>
      <View style={styles.section}>
        <Text style={styles.heading} accessibilityRole="header">
          {t('ui.language')}
        </Text>
        <LanguageToggle />
      </View>

      <View style={styles.section}>
        <Text style={styles.heading} accessibilityRole="header">
          {t('mobile.accountHeading')}
        </Text>
        <Text style={styles.body}>{t('mobile.accountBody')}</Text>
        <Pressable
          onPress={confirmDelete}
          disabled={deleteState === 'deleting'}
          style={[styles.danger, deleteState === 'deleting' && styles.disabled]}
          accessibilityRole="button"
          accessibilityState={{ disabled: deleteState === 'deleting', busy: deleteState === 'deleting' }}
        >
          {deleteState === 'deleting' ? (
            <ActivityIndicator color="#fff" accessibilityLabel={t('mobile.deleting')} />
          ) : (
            <Text style={styles.dangerText}>{t('mobile.deleteAccount')}</Text>
          )}
        </Pressable>
        {deleteState === 'deleted' && (
          <Text style={styles.body} accessibilityLiveRegion="polite">
            {t('mobile.accountDeleted')}
          </Text>
        )}
        {deleteState === 'failed' && (
          <Text style={styles.error} accessibilityLiveRegion="polite">
            {t('mobile.deleteFailed')}
          </Text>
        )}
      </View>

      <View style={styles.section}>
        <Text style={styles.heading} accessibilityRole="header">
          {t('mobile.aboutHeading')}
        </Text>
        <Text style={styles.meta}>{t('mobile.version', { version: Constants.expoConfig?.version ?? '—' })}</Text>
        <Text style={styles.subheading}>{t('mobile.dataSources')}</Text>
        <Text style={styles.link} accessibilityRole="link" onPress={() => Linking.openURL(WEATHER_ATTRIBUTION.url)}>
          {t('weather.attribution')} ({WEATHER_ATTRIBUTION.licence})
        </Text>
        <Text style={styles.link} accessibilityRole="link" onPress={() => Linking.openURL(OSM_COPYRIGHT_URL)}>
          {t('mobile.mapCredit')}
        </Text>
        <Text style={styles.notice}>{t('notice.decisionSupport')}</Text>
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  section: { gap: 10 },
  heading: { fontSize: 20, fontWeight: '600' },
  subheading: { fontSize: 16, fontWeight: '600', marginTop: 4 },
  body: { fontSize: 14, lineHeight: 20 },
  meta: { fontSize: 13, color: '#555' },
  link: { fontSize: 14, lineHeight: 20, color: '#0b5cad' },
  error: { color: '#b00020' },
  notice: { fontSize: 12, lineHeight: 17, color: '#444', backgroundColor: '#fff8e1', padding: 8, borderRadius: 6 },
  danger: { alignSelf: 'flex-start', backgroundColor: '#b00020', paddingVertical: 10, paddingHorizontal: 16, borderRadius: 6 },
  disabled: { opacity: 0.6 },
  dangerText: { color: '#fff', fontWeight: '600' },
});
