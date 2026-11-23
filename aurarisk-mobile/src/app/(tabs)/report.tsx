import LocationPicker from '../../components/LocationPicker';
import QueuedReports from '../../components/QueuedReports';
import ReportComposer from '../../components/ReportComposer';
import Screen from '../../components/Screen';
import { useAppState } from '../../state/AppState';

/** Report tab: reports waiting on this device, then a new report for the selected location. */
export default function ReportScreen() {
  const { location, setLocation, onReportSubmitted, sync } = useAppState();
  return (
    <Screen showsQueue>
      <QueuedReports queue={sync.queue} onRequeued={sync.syncPendingReports} />
      <LocationPicker value={location} onChange={setLocation} />
      <ReportComposer location={location} onSubmitted={onReportSubmitted} />
    </Screen>
  );
}
