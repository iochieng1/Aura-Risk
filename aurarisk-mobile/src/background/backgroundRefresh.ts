import * as BackgroundTask from 'expo-background-task';
import * as Battery from 'expo-battery';
import * as TaskManager from 'expo-task-manager';
import { getCachedSubscriptions } from '../notifications/subscriptions';
import { fetchRiskAssessment, isOnline } from '../services/api';
import { flushReportQueue } from '../sync/flushReportQueue';

export const BACKGROUND_REFRESH_TASK = 'aurarisk-background-refresh';

// WorkManager's floor is 15 minutes; iOS treats this as a hint and schedules by its own budget.
const MINIMUM_INTERVAL_MINUTES = 15;

// iOS gives BGTaskScheduler work roughly 30s. Stop well before that.
const TIME_BUDGET_MS = 20_000;
const MAX_LOCATIONS_PER_RUN = 10;

const withDeadline = <T,>(promise: Promise<T>, deadline: number): Promise<T | 'timeout'> => {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<'timeout'>((resolve) => {
    timer = setTimeout(() => resolve('timeout'), Math.max(0, deadline - Date.now()));
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
};

export type RefreshOutcome = 'skipped_low_power' | 'skipped_offline' | 'completed' | 'failed';

/** One background run: flush queued reports, then refresh cached risk for followed locations. */
export const runBackgroundRefresh = async (): Promise<RefreshOutcome> => {
  const deadline = Date.now() + TIME_BUDGET_MS;

  // Low Power Mode / Battery Saver means the user asked apps to cut back; honor it.
  if (await Battery.isLowPowerModeEnabledAsync().catch(() => false)) return 'skipped_low_power';
  if (!(await isOnline())) return 'skipped_offline';

  let failures = 0;
  let attempts = 0;

  attempts += 1;
  try {
    const flushed = await withDeadline(flushReportQueue(), deadline);
    if (flushed === 'timeout') return 'completed';
  } catch (err) {
    failures += 1;
    console.warn('Background report flush failed:', err);
  }

  const subscriptions = (await getCachedSubscriptions()).slice(0, MAX_LOCATIONS_PER_RUN);
  for (const subscription of subscriptions) {
    if (Date.now() >= deadline) break;
    attempts += 1;
    const result = await withDeadline(fetchRiskAssessment(subscription.lat, subscription.lon), deadline);
    if (result === 'timeout') break;
    if (result.isOffline) failures += 1;
  }

  return failures > 0 && failures === attempts ? 'failed' : 'completed';
};

// Must be defined in the global scope: on a background launch no React tree is mounted.
TaskManager.defineTask(BACKGROUND_REFRESH_TASK, async () => {
  try {
    const outcome = await runBackgroundRefresh();
    return outcome === 'failed' ? BackgroundTask.BackgroundTaskResult.Failed : BackgroundTask.BackgroundTaskResult.Success;
  } catch (err) {
    console.error('Background refresh failed:', err);
    return BackgroundTask.BackgroundTaskResult.Failed;
  }
});

export const registerBackgroundRefreshAsync = async (): Promise<boolean> => {
  const status = await BackgroundTask.getStatusAsync();
  if (status !== BackgroundTask.BackgroundTaskStatus.Available) return false;
  if (await TaskManager.isTaskRegisteredAsync(BACKGROUND_REFRESH_TASK)) return true;
  await BackgroundTask.registerTaskAsync(BACKGROUND_REFRESH_TASK, { minimumInterval: MINIMUM_INTERVAL_MINUTES });
  return true;
};

export const unregisterBackgroundRefreshAsync = async (): Promise<void> => {
  if (await TaskManager.isTaskRegisteredAsync(BACKGROUND_REFRESH_TASK)) {
    await BackgroundTask.unregisterTaskAsync(BACKGROUND_REFRESH_TASK);
  }
};
