import AsyncStorage from '@react-native-async-storage/async-storage';
import { authFetch } from '../auth/authClient';
import { Subscription, SubscriptionInput } from '../types/api';

// Subscriptions are not secrets; caching them lets the background task refresh risk for these
// locations and lets the app restore them if the device account has to be re-created.
const CACHE_KEY = '@aurarisk/subscriptions';

export const MAX_SUBSCRIPTIONS = 10;

export const getCachedSubscriptions = async (): Promise<Subscription[]> => {
  try {
    const raw = await AsyncStorage.getItem(CACHE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
};

const saveCache = async (subscriptions: Subscription[]) => {
  try {
    await AsyncStorage.setItem(CACHE_KEY, JSON.stringify(subscriptions));
  } catch (err) {
    console.warn('Failed to cache subscriptions:', err);
  }
};

export const clearCachedSubscriptions = () => AsyncStorage.removeItem(CACHE_KEY);

const readError = async (response: Response, action: string) => {
  let detail = '';
  try {
    const body = await response.json();
    detail = body?.error ? `: ${body.error}` : '';
  } catch {
    // Non-JSON error body.
  }
  return new Error(`Failed to ${action} subscription (${response.status})${detail}`);
};

export const listSubscriptions = async (): Promise<Subscription[]> => {
  const response = await authFetch('/api/subscriptions');
  if (!response.ok) throw await readError(response, 'list');
  const subscriptions: Subscription[] = await response.json();
  await saveCache(subscriptions);
  return subscriptions;
};

export const createSubscription = async (input: SubscriptionInput): Promise<Subscription> => {
  const response = await authFetch('/api/subscriptions', { method: 'POST', body: JSON.stringify(input) });
  if (!response.ok) throw await readError(response, 'create');
  const created: Subscription = await response.json();
  await saveCache([...(await getCachedSubscriptions()).filter((s) => s.id !== created.id), created]);
  return created;
};

export const updateSubscription = async (id: string, input: SubscriptionInput): Promise<Subscription> => {
  const response = await authFetch(`/api/subscriptions/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await readError(response, 'update');
  const updated: Subscription = await response.json();
  await saveCache((await getCachedSubscriptions()).map((s) => (s.id === id ? updated : s)));
  return updated;
};

export const deleteSubscription = async (id: string): Promise<void> => {
  const response = await authFetch(`/api/subscriptions/${encodeURIComponent(id)}`, { method: 'DELETE' });
  // 404 means it is already gone server-side; drop it locally either way.
  if (!response.ok && response.status !== 404) throw await readError(response, 'delete');
  await saveCache((await getCachedSubscriptions()).filter((s) => s.id !== id));
};

const toInput = (s: Subscription): SubscriptionInput => ({
  name: s.name,
  lat: s.lat,
  lon: s.lon,
  min_level: s.min_level,
  quiet_hours: s.quiet_hours,
  allow_emergency_during_quiet_hours: s.allow_emergency_during_quiet_hours,
});

/** Re-creates cached subscriptions under a new device account (after a forced re-registration). */
export const restoreCachedSubscriptions = async (): Promise<void> => {
  const cached = await getCachedSubscriptions();
  if (cached.length === 0) return;
  await clearCachedSubscriptions();
  for (const subscription of cached) {
    try {
      await createSubscription(toInput(subscription));
    } catch (err) {
      console.warn('Failed to restore subscription:', err);
    }
  }
};
