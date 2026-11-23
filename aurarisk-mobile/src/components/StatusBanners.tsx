import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useT } from '../i18n';
import { useServerWaking } from '../net/serverStatus';
import { useAppState } from '../state/AppState';

interface Props {
  /**
   * Opens the screen that lists unsent reports. When set, the "not sent" banner is a button that
   * calls it; leave it unset on that screen itself, where the list is already shown.
   */
  onReviewFailed?: () => void;
}

/** Server wake-up, queued-report and failed-report banners, shown at the top of every tab. */
export default function StatusBanners({ onReviewFailed }: Props) {
  const { t } = useT();
  const serverWaking = useServerWaking();
  const { pendingCount, failedCount, isSyncing } = useAppState().sync;

  if (!serverWaking && pendingCount === 0 && !isSyncing && failedCount === 0) return null;

  return (
    <View style={styles.container}>
      {serverWaking && (
        <Text style={styles.banner} accessibilityLiveRegion="polite">
          {t('mobile.serverWaking')}
        </Text>
      )}
      {(pendingCount > 0 || isSyncing) && (
        <Text style={styles.banner}>
          {isSyncing ? t('mobile.sendingQueued') : t('mobile.waiting', { count: pendingCount })}
        </Text>
      )}
      {failedCount > 0 &&
        (onReviewFailed ? (
          <Pressable onPress={onReviewFailed} accessibilityRole="button" accessibilityLiveRegion="polite">
            <Text style={[styles.banner, styles.failedBanner]}>{t('mobile.notSentReview', { count: failedCount })}</Text>
          </Pressable>
        ) : (
          <Text style={[styles.banner, styles.failedBanner]} accessibilityLiveRegion="polite">
            {t('mobile.notSent', { count: failedCount })}
          </Text>
        ))}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: 8 },
  banner: { backgroundColor: '#fff4ce', padding: 10, borderRadius: 6 },
  failedBanner: { backgroundColor: '#fde7e9' },
});
