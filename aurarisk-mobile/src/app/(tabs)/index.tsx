import type { Location } from '@aurarisk/shared';
import { router, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { StyleSheet, Text } from 'react-native';
import LocationPicker from '../../components/LocationPicker';
import RiskPanel from '../../components/RiskPanel';
import Screen from '../../components/Screen';
import { useT } from '../../i18n';
import { resolveRiskLink, type RiskLinkParams } from '../../navigation/riskLink';
import { getCachedSubscriptions } from '../../notifications/subscriptions';
import { useAppState } from '../../state/AppState';

/** Risk tab: pick a location, see its flood risk and nearby reports. Also the target of alert taps and deep links. */
export default function RiskScreen() {
  const { t } = useT();
  const { location, setLocation, refreshKey } = useAppState();
  const params = useLocalSearchParams<RiskLinkParams & Record<string, string | string[]>>();
  // The location an alert or link opened; the notice shows until the user picks another one.
  const [linked, setLinked] = useState<Location | null>(null);

  const { subscription, lat, lon } = params;
  const linkKey = JSON.stringify([subscription, lat, lon]);
  useEffect(() => {
    if (subscription === undefined && lat === undefined && lon === undefined) return;
    let cancelled = false;
    getCachedSubscriptions().then((subscriptions) => {
      if (cancelled) return;
      const resolved = resolveRiskLink({ subscription, lat, lon }, subscriptions, t('mobile.alertLocation'));
      if (!resolved) return;
      setLinked(resolved.location);
      setLocation(resolved.location);
    }).finally(() => {
      // Consumed: clear the params so tapping the same alert again later is seen as a change.
      if (!cancelled) router.setParams({ subscription: undefined, lat: undefined, lon: undefined });
    });
    return () => {
      cancelled = true;
    };
    // linkKey stands in for the params. A language change must not re-resolve a link already handled.
  }, [linkKey]);

  return (
    <Screen>
      {linked && linked === location && (
        <Text style={styles.notice} accessibilityLiveRegion="polite">
          {t('mobile.openedFromAlert', { name: linked.name })}
        </Text>
      )}
      <LocationPicker value={location} onChange={setLocation} />
      {location && <RiskPanel location={location} refreshKey={refreshKey} />}
    </Screen>
  );
}

const styles = StyleSheet.create({
  notice: { backgroundColor: '#e8f1fb', padding: 10, borderRadius: 6 },
});
