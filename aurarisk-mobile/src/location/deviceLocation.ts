import type { Location as Place } from '@aurarisk/shared';
import * as Location from 'expo-location';
import { Platform } from 'react-native';

// A recent cached fix is good enough for a flood-risk lookup and avoids waiting on GPS.
const LAST_KNOWN_MAX_AGE_MS = 2 * 60 * 1000;
const LAST_KNOWN_REQUIRED_ACCURACY_M = 500;
const CURRENT_POSITION_TIMEOUT_MS = 15_000;

export type LocationPermission = 'granted' | 'undetermined' | 'denied' | 'blocked';

export type DeviceLocationResult =
  | { status: 'ok'; location: Place; accuracyMeters: number | null }
  /** `blocked` means the OS will not show the prompt again; only Settings can change it. */
  | { status: 'denied'; blocked: boolean }
  | { status: 'services_disabled' }
  | { status: 'unavailable'; message: string };

const toPermission = (p: Location.LocationPermissionResponse): LocationPermission => {
  if (p.granted) return 'granted';
  if (p.status === Location.PermissionStatus.UNDETERMINED) return 'undetermined';
  return p.canAskAgain ? 'denied' : 'blocked';
};

/** Reads the current permission without prompting. */
export const getLocationPermission = async (): Promise<LocationPermission> => {
  try {
    return toPermission(await Location.getForegroundPermissionsAsync());
  } catch {
    return 'blocked';
  }
};

const withTimeout = <T>(promise: Promise<T>, ms: number): Promise<T> =>
  new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Timed out waiting for a location fix')), ms);
    promise.then(
      (v) => {
        clearTimeout(timer);
        resolve(v);
      },
      (e) => {
        clearTimeout(timer);
        reject(e);
      }
    );
  });

export const formatCoordinates = (lat: number, lon: number) => `${lat.toFixed(4)}, ${lon.toFixed(4)}`;

/** Builds a short, human-readable label from a reverse-geocoded address. */
export const formatAddress = (address: Location.LocationGeocodedAddress): string | null => {
  if (address.formattedAddress) return address.formattedAddress;
  const parts = [address.name, address.street, address.district, address.city, address.region].filter(
    (p): p is string => Boolean(p && p.trim())
  );
  const unique = parts.filter((p, i) => parts.indexOf(p) === i);
  return unique.length ? unique.slice(0, 3).join(', ') : null;
};

const describePlace = async (lat: number, lon: number): Promise<string> => {
  const fallback = `Current location (${formatCoordinates(lat, lon)})`;
  // reverseGeocodeAsync is Android/iOS only.
  if (Platform.OS === 'web') return fallback;
  try {
    const [address] = await Location.reverseGeocodeAsync({ latitude: lat, longitude: lon });
    return (address && formatAddress(address)) || fallback;
  } catch {
    return fallback;
  }
};

/**
 * Asks for foreground ("while using the app") permission if it has not been decided, then returns
 * the device's position. Never throws: every outcome maps to a result the UI can act on, so the
 * caller can fall back to manual selection.
 */
export const getDeviceLocation = async (): Promise<DeviceLocationResult> => {
  try {
    let permission = await Location.getForegroundPermissionsAsync();
    if (!permission.granted && permission.canAskAgain) {
      permission = await Location.requestForegroundPermissionsAsync();
    }
    if (!permission.granted) return { status: 'denied', blocked: !permission.canAskAgain };

    if (!(await Location.hasServicesEnabledAsync())) return { status: 'services_disabled' };

    const position =
      (await Location.getLastKnownPositionAsync({
        maxAge: LAST_KNOWN_MAX_AGE_MS,
        requiredAccuracy: LAST_KNOWN_REQUIRED_ACCURACY_M,
      }).catch(() => null)) ??
      (await withTimeout(
        Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced }),
        CURRENT_POSITION_TIMEOUT_MS
      ));

    const { latitude: lat, longitude: lon, accuracy } = position.coords;
    return {
      status: 'ok',
      location: { name: await describePlace(lat, lon), lat, lon },
      accuracyMeters: accuracy ?? null,
    };
  } catch (err) {
    return { status: 'unavailable', message: err instanceof Error ? err.message : 'Location unavailable' };
  }
};
