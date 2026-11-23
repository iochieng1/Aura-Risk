import { router } from 'expo-router';
import type { ReactNode } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import StatusBanners from './StatusBanners';

interface Props {
  children: ReactNode;
  /** False for screens that lay out their own content to fill the space, like the map. */
  scroll?: boolean;
  /** True on the screen that lists unsent reports, so the banner doesn't link to itself. */
  showsQueue?: boolean;
}

const openReportTab = () => router.navigate('/report');

/** A tab's body: the shared status banners, then the screen's content. */
export default function Screen({ children, scroll = true, showsQueue = false }: Props) {
  const banners = <StatusBanners onReviewFailed={showsQueue ? undefined : openReportTab} />;

  if (!scroll) {
    return (
      <View style={styles.fill}>
        <View style={styles.fixedBanners}>{banners}</View>
        {children}
      </View>
    );
  }
  return (
    <ScrollView style={styles.fill} contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      {banners}
      {children}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: '#fff' },
  content: { padding: 16, gap: 32 },
  fixedBanners: { paddingHorizontal: 16, paddingTop: 8 },
});
