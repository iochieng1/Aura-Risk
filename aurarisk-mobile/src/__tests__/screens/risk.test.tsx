import type { Location } from '@aurarisk/shared';
import { render, screen, waitFor } from '@testing-library/react-native';
import { I18nProvider } from '../../i18n';
import RiskScreen from '../../app/(tabs)/index';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

let mockParams: Record<string, string> = {};
const mockSetParams = jest.fn();
jest.mock('expo-router', () => ({
  useNavigation: () => ({ setParams: (...args: unknown[]) => mockSetParams(...args) }),
  useLocalSearchParams: () => mockParams,
}));

jest.mock('../../components/LocationPicker', () => () => null);
jest.mock('../../components/RiskPanel', () => () => null);
jest.mock('../../components/StatusBanners', () => () => null);

jest.mock('../../notifications/subscriptions', () => ({
  getCachedSubscriptions: async () => [{ id: 'sub-1', name: 'Mathare', lat: -1.26, lon: 36.86 }],
}));

// A real state holder so the notice can compare the linked location with the current one.
let mockLocation: Location | null = null;
const mockSetLocation = jest.fn((l: Location) => {
  mockLocation = l;
});
jest.mock('../../state/AppState', () => ({
  useAppState: () => ({ location: mockLocation, setLocation: mockSetLocation, refreshKey: 0 }),
}));

// The first render pays for loading React Native's renderer, which can exceed 5 s on a cold CI cache.
jest.setTimeout(20_000);

const renderRisk = () =>
  render(
    <I18nProvider>
      <RiskScreen />
    </I18nProvider>
  );

beforeEach(() => {
  mockLocation = null;
  mockParams = {};
  mockSetLocation.mockClear();
  mockSetParams.mockClear();
});

describe('Risk tab deep links', () => {
  it('opens the followed location named by a subscription link', async () => {
    mockParams = { subscription: 'sub-1' };
    renderRisk();
    await waitFor(() => expect(mockSetLocation).toHaveBeenCalled());
    expect(mockSetLocation.mock.calls[0][0]).toMatchObject({ name: 'Mathare', lat: -1.26, lon: 36.86 });
    expect(mockSetParams).toHaveBeenCalledWith({ subscription: undefined, lat: undefined, lon: undefined });

    screen.rerender(
      <I18nProvider>
        <RiskScreen />
      </I18nProvider>
    );
    expect(screen.getByText('Opened from a flood alert for Mathare.')).toBeTruthy();
  });

  it('ignores a link with nothing usable in it', async () => {
    mockParams = { subscription: 'unknown' };
    renderRisk();
    await waitFor(() => expect(mockSetParams).toHaveBeenCalled());
    expect(mockSetLocation).not.toHaveBeenCalled();
    expect(screen.queryByText(/Opened from a flood alert/)).toBeNull();
  });

  it('does nothing on a plain visit', async () => {
    renderRisk();
    await new Promise((r) => setTimeout(r, 0));
    expect(mockSetLocation).not.toHaveBeenCalled();
    expect(mockSetParams).not.toHaveBeenCalled();
  });
});
