import * as Location from 'expo-location';
import { formatAddress, getDeviceLocation } from '../deviceLocation';

jest.mock('expo-location', () => ({
  PermissionStatus: { GRANTED: 'granted', UNDETERMINED: 'undetermined', DENIED: 'denied' },
  Accuracy: { Balanced: 3 },
  getForegroundPermissionsAsync: jest.fn(),
  requestForegroundPermissionsAsync: jest.fn(),
  hasServicesEnabledAsync: jest.fn(),
  getLastKnownPositionAsync: jest.fn(),
  getCurrentPositionAsync: jest.fn(),
  reverseGeocodeAsync: jest.fn(),
}));

const mocked = Location as jest.Mocked<typeof Location>;

const permission = (granted: boolean, canAskAgain: boolean, status = granted ? 'granted' : 'denied') =>
  ({ granted, canAskAgain, status, expires: 'never' }) as unknown as Location.LocationPermissionResponse;

const position = (latitude: number, longitude: number) =>
  ({ coords: { latitude, longitude, accuracy: 25 }, timestamp: Date.now() }) as Location.LocationObject;

const address = (fields: Partial<Location.LocationGeocodedAddress>) =>
  ({
    city: null,
    country: null,
    district: null,
    formattedAddress: null,
    isoCountryCode: null,
    name: null,
    postalCode: null,
    region: null,
    street: null,
    streetNumber: null,
    subregion: null,
    timezone: null,
    ...fields,
  }) as Location.LocationGeocodedAddress;

describe('getDeviceLocation', () => {
  beforeEach(() => {
    jest.resetAllMocks();
    mocked.hasServicesEnabledAsync.mockResolvedValue(true);
    mocked.getLastKnownPositionAsync.mockResolvedValue(null);
    mocked.reverseGeocodeAsync.mockResolvedValue([]);
  });

  it('prompts when undecided and returns the position', async () => {
    mocked.getForegroundPermissionsAsync.mockResolvedValue(permission(false, true, 'undetermined'));
    mocked.requestForegroundPermissionsAsync.mockResolvedValue(permission(true, true));
    mocked.getCurrentPositionAsync.mockResolvedValue(position(-1.2921, 36.8219));
    mocked.reverseGeocodeAsync.mockResolvedValue([address({ district: 'Westlands', city: 'Nairobi' })]);

    await expect(getDeviceLocation()).resolves.toEqual({
      status: 'ok',
      location: { name: 'Westlands, Nairobi', lat: -1.2921, lon: 36.8219 },
      accuracyMeters: 25,
    });
  });

  it('does not prompt again once the user has blocked it', async () => {
    mocked.getForegroundPermissionsAsync.mockResolvedValue(permission(false, false));

    await expect(getDeviceLocation()).resolves.toEqual({ status: 'denied', blocked: true });
    expect(mocked.requestForegroundPermissionsAsync).not.toHaveBeenCalled();
  });

  it('reports a soft denial so the app may ask again later', async () => {
    mocked.getForegroundPermissionsAsync.mockResolvedValue(permission(false, true, 'undetermined'));
    mocked.requestForegroundPermissionsAsync.mockResolvedValue(permission(false, true));

    await expect(getDeviceLocation()).resolves.toEqual({ status: 'denied', blocked: false });
  });

  it('detects disabled location services', async () => {
    mocked.getForegroundPermissionsAsync.mockResolvedValue(permission(true, true));
    mocked.hasServicesEnabledAsync.mockResolvedValue(false);

    await expect(getDeviceLocation()).resolves.toEqual({ status: 'services_disabled' });
  });

  it('prefers a recent last-known fix and falls back to coordinates when geocoding fails', async () => {
    mocked.getForegroundPermissionsAsync.mockResolvedValue(permission(true, true));
    mocked.getLastKnownPositionAsync.mockResolvedValue(position(10, 20));
    mocked.reverseGeocodeAsync.mockRejectedValue(new Error('rate limited'));

    const result = await getDeviceLocation();

    expect(mocked.getCurrentPositionAsync).not.toHaveBeenCalled();
    expect(result).toMatchObject({ status: 'ok', location: { name: 'Current location (10.0000, 20.0000)' } });
  });

  it('maps a failed fix to unavailable instead of throwing', async () => {
    mocked.getForegroundPermissionsAsync.mockResolvedValue(permission(true, true));
    mocked.getCurrentPositionAsync.mockRejectedValue(new Error('No fix'));

    await expect(getDeviceLocation()).resolves.toEqual({ status: 'unavailable', message: 'No fix' });
  });
});

describe('formatAddress', () => {
  it('prefers the platform-formatted address', () => {
    expect(formatAddress(address({ formattedAddress: '1 Main St', city: 'X' }))).toBe('1 Main St');
  });

  it('drops duplicate parts and returns null when empty', () => {
    expect(formatAddress(address({ name: 'Nairobi', city: 'Nairobi', region: 'Nairobi County' }))).toBe(
      'Nairobi, Nairobi County'
    );
    expect(formatAddress(address({}))).toBeNull();
  });
});
