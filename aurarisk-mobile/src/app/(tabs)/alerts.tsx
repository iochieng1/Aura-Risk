import NotificationSettings from '../../components/NotificationSettings';
import Screen from '../../components/Screen';
import { useAppState } from '../../state/AppState';

/** Alerts tab: push consent and the locations this device follows. */
export default function AlertsScreen() {
  // Remount after account deletion so the deleted account's followed locations and push state are not shown.
  const { accountGeneration } = useAppState();
  return (
    <Screen>
      <NotificationSettings key={accountGeneration} />
    </Screen>
  );
}
