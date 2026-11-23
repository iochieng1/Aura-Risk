import { type Location, validateLocation } from '@aurarisk/shared';
import type { RiskAlertData, Subscription } from '../types/api';

/**
 * Search params the Risk tab (`/`) accepts, from a notification tap or a deep link such as
 * `aurarisk:///?subscription=<id>` or `aurarisk:///?lat=-1.29&lon=36.82`. Expo Router may hand a
 * repeated param over as an array, so every field allows one.
 */
export interface RiskLinkParams {
  subscription?: string | string[];
  lat?: string | string[];
  lon?: string | string[];
}

/** Where a tapped risk alert should open: the Risk tab for that subscription's location. */
export const riskAlertHref = (alert: RiskAlertData) => ({
  pathname: '/' as const,
  params: { subscription: alert.subscription_id, lat: String(alert.lat), lon: String(alert.lon) },
});

const first = (value: string | string[] | undefined): string | undefined =>
  Array.isArray(value) ? value[0] : value;

const toNumber = (value: string | undefined): number | undefined => {
  if (value === undefined || value.trim() === '') return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
};

export interface ResolvedRiskLink {
  location: Location;
  /** The followed location the link is about, when this device still has it. */
  subscription?: Subscription;
}

/**
 * The location a Risk link points at, or null when it names nothing usable. Coordinates in the link
 * win (they are what the alert was about); a subscription id alone is looked up in `subscriptions`.
 * `fallbackName` names a location the device no longer follows.
 */
export const resolveRiskLink = (
  params: RiskLinkParams,
  subscriptions: Subscription[],
  fallbackName: string
): ResolvedRiskLink | null => {
  const id = first(params.subscription);
  const subscription = id ? subscriptions.find((s) => s.id === id) : undefined;
  const lat = toNumber(first(params.lat)) ?? subscription?.lat;
  const lon = toNumber(first(params.lon)) ?? subscription?.lon;
  if (lat === undefined || lon === undefined) return null;

  const checked = validateLocation({ name: subscription?.name ?? fallbackName, lat, lon });
  if (!checked.ok) return null;
  return { location: checked.value, subscription };
};
