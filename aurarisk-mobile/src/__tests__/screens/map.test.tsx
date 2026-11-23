import type { CommunityReport } from '@aurarisk/shared';
import { fireEvent, render, screen } from '@testing-library/react-native';
import { I18nProvider } from '../../i18n';
import MapScreen from '../../app/(tabs)/map';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

const mockNavigate = jest.fn();
jest.mock('expo-router', () => ({ router: { navigate: (...args: unknown[]) => mockNavigate(...args) } }));

// The native map can't render under Jest; the list view is what these tests exercise.
jest.mock('../../components/ReportsMap', () => () => null);
jest.mock('../../components/StatusBanners', () => () => null);

const appState: { location: unknown; refreshKey: number } = { location: null, refreshKey: 0 };
jest.mock('../../state/AppState', () => ({ useAppState: () => appState }));

const nearby = { reports: [] as CommunityReport[], isOffline: false, loading: false };
jest.mock('../../hooks/useNearbyReports', () => ({ useNearbyReports: () => nearby }));

const report: CommunityReport = {
  id: 'r1',
  location: { name: 'Kibera', lat: -1.31, lon: 36.78 },
  category: 'flooding',
  note: 'Water over the footbridge',
  timestamp: '2026-11-20T08:00:00Z',
  status: 'verified',
  county: 'Nairobi',
  ward: 'Laini Saba',
} as CommunityReport;

const renderMap = () =>
  render(
    <I18nProvider>
      <MapScreen />
    </I18nProvider>
  );

beforeEach(() => {
  appState.location = { name: 'Kibera', lat: -1.31, lon: 36.78 };
  Object.assign(nearby, { reports: [report], isOffline: false, loading: false });
  mockNavigate.mockClear();
});

describe('Map tab', () => {
  it('asks for a location first and links to the Risk tab', () => {
    appState.location = null;
    renderMap();
    expect(screen.getByText(/Choose a location on the Risk tab/)).toBeTruthy();
    fireEvent.press(screen.getByRole('button', { name: 'Choose a location' }));
    expect(mockNavigate).toHaveBeenCalledWith('/');
  });

  it('shows the same reports as a list', () => {
    renderMap();
    expect(screen.queryByText('Water over the footbridge')).toBeNull();
    fireEvent.press(screen.getByRole('radio', { name: 'List' }));
    expect(screen.getByText('Water over the footbridge')).toBeTruthy();
    expect(screen.getByText(/Laini Saba, Nairobi/)).toBeTruthy();
  });

  it('says when reports come from the offline cache', () => {
    nearby.isOffline = true;
    renderMap();
    expect(screen.getByText(/Map areas you have not viewed before may be blank/)).toBeTruthy();
    fireEvent.press(screen.getByRole('radio', { name: 'List' }));
    expect(screen.getByText('Offline: showing reports saved on this device.')).toBeTruthy();
  });

  it('works in Swahili', async () => {
    renderMap();
    // I18nProvider reads a saved override on mount; switch through the same storage key.
    const AsyncStorage = require('@react-native-async-storage/async-storage');
    await AsyncStorage.setItem('aurarisk.lang', 'sw');
    screen.unmount();
    renderMap();
    expect(await screen.findByText('Ripoti kwenye ramani')).toBeTruthy();
    expect(screen.getByRole('radio', { name: 'Orodha' })).toBeTruthy();
    await AsyncStorage.removeItem('aurarisk.lang');
  });
});
