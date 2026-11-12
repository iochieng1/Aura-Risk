import AsyncStorage from '@react-native-async-storage/async-storage';
import { NewReportInput } from '../types/api';

const QUEUE_KEY = '@aurarisk/offline_reports_queue';
/** Where an unreadable queue is copied before anything overwrites it, so it can still be recovered. */
export const CORRUPT_QUEUE_KEY = '@aurarisk/offline_reports_queue_corrupt';

/**
 * - `pending`: sent automatically, no earlier than `nextAttemptAt`.
 * - `failed`: the server rejected it (4xx). Never retried automatically; the user retries, edits or
 *   discards it.
 *
 * A report leaves the queue only when it has been sent (`dequeueReport`) or the user discards it
 * (`discardQueuedReport`). Nothing else removes it.
 */
export type QueuedReportStatus = 'pending' | 'failed';

export interface QueuedReport extends NewReportInput {
  tempId: string;
  status: QueuedReportStatus;
  /** Automatic sends that failed for a transient reason (network, timeout, 5xx). Drives backoff only. */
  attempts: number;
  /** Epoch ms. A pending report is not sent automatically before this. Unset means as soon as possible. */
  nextAttemptAt?: number;
  /** Why the last attempt failed. For a failed report this is the server's rejection reason. */
  lastError?: string;
  createdAt: string;
  /** Local file URIs (app document directory) still waiting to be uploaded. */
  photos: string[];
  /** Set once the report exists on the server, so retries only upload the remaining photos. */
  serverReportId?: string;
}

export type NewReport = NewReportInput;

// Automatic retry schedule after consecutive transient failures:
// 30 s, 1, 2, 4, 8, 16, 32 min, then every hour until it is sent.
export const RETRY_BASE_DELAY_MS = 30_000;
export const RETRY_MAX_DELAY_MS = 60 * 60_000;

/** Delay before the next automatic attempt after `attempts` consecutive transient failures. */
export const backoffDelayMs = (attempts: number): number =>
  Math.min(RETRY_BASE_DELAY_MS * 2 ** Math.max(0, attempts - 1), RETRY_MAX_DELAY_MS);

/** Whether an automatic flush should try this report now. Failed reports wait for the user. */
export const isDue = (item: QueuedReport, now = Date.now()): boolean =>
  item.status === 'pending' && (item.nextAttemptAt === undefined || item.nextAttemptAt <= now);

const newTempId = () => `pending_${Date.now()}_${Math.random().toString(36).substring(2, 7)}`;

/** Brings items stored by older app versions up to the current shape. Never drops a report. */
const normalize = (raw: unknown): QueuedReport | null => {
  if (!raw || typeof raw !== 'object') return null;
  const { retryCount, ...item } = raw as Partial<QueuedReport> & { retryCount?: unknown };
  let attempts = 0;
  if (typeof item.attempts === 'number') attempts = item.attempts;
  else if (typeof retryCount === 'number') attempts = retryCount; // before #80 the field was retryCount
  return {
    ...(item as QueuedReport),
    tempId: typeof item.tempId === 'string' && item.tempId ? item.tempId : newTempId(),
    status: item.status === 'failed' ? 'failed' : 'pending',
    attempts,
    createdAt: typeof item.createdAt === 'string' ? item.createdAt : new Date().toISOString(),
    // Items queued before photo support have no `photos` field.
    photos: Array.isArray(item.photos) ? item.photos : [],
  };
};

const readQueue = async (): Promise<QueuedReport[]> => {
  const raw = await AsyncStorage.getItem(QUEUE_KEY);
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    // Keep the unreadable copy: the next write would otherwise destroy the only record of these reports.
    console.error(`Report queue is unreadable; a copy is kept under ${CORRUPT_QUEUE_KEY}`);
    await AsyncStorage.setItem(CORRUPT_QUEUE_KEY, raw).catch(() => undefined);
    return [];
  }
  if (!Array.isArray(parsed)) return [];
  return parsed.map(normalize).filter((item): item is QueuedReport => item !== null);
};

type Listener = (queue: QueuedReport[]) => void;
const listeners = new Set<Listener>();

/** Called with the whole queue after every change, so counts shown in the UI come from the queue itself. */
export const subscribeToReportQueue = (listener: Listener): (() => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

// Every change is a read-modify-write. Running them one at a time stops a report enqueued during a
// flush from being overwritten by the flush's own write.
let lock: Promise<unknown> = Promise.resolve();

const mutate = <T>(change: (queue: QueuedReport[]) => { queue: QueuedReport[]; result: T }): Promise<T> => {
  const run = lock.then(async () => {
    const { queue, result } = change(await readQueue());
    await AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(queue));
    listeners.forEach((listener) => listener(queue));
    return result;
  });
  lock = run.catch(() => undefined);
  return run;
};

export const getReportQueue = async (): Promise<QueuedReport[]> => {
  try {
    await lock;
    return await readQueue();
  } catch {
    return [];
  }
};

export interface QueueCounts {
  pending: number;
  failed: number;
}

export const countQueue = (queue: QueuedReport[]): QueueCounts => ({
  pending: queue.filter((item) => item.status === 'pending').length,
  failed: queue.filter((item) => item.status === 'failed').length,
});

const updateItem = (tempId: string, update: (item: QueuedReport) => QueuedReport): Promise<void> =>
  mutate((queue) => ({ queue: queue.map((item) => (item.tempId === tempId ? update(item) : item)), result: undefined }));

export const enqueueReport = (report: NewReport, photos: string[] = [], serverReportId?: string): Promise<QueuedReport> => {
  const queuedItem: QueuedReport = {
    ...report,
    tempId: newTempId(),
    status: 'pending',
    attempts: 0,
    createdAt: new Date().toISOString(),
    photos,
    ...(serverReportId ? { serverReportId } : {}),
  };
  return mutate((queue) => ({ queue: [...queue, queuedItem], result: queuedItem }));
};

/** Removes a report once it has been sent. A user's discard goes through `discardQueuedReport`. */
export const dequeueReport = (tempId: string): Promise<void> =>
  mutate((queue) => ({ queue: queue.filter((item) => item.tempId !== tempId), result: undefined }));

/** Removes a report the user chose to discard. Returns its local photo URIs so the caller can delete them. */
export const discardQueuedReport = (tempId: string): Promise<string[]> =>
  mutate((queue) => ({
    queue: queue.filter((item) => item.tempId !== tempId),
    result: queue.find((item) => item.tempId === tempId)?.photos ?? [],
  }));

/**
 * A transient failure: the report stays pending and is tried again after the backoff delay, or
 * later if the server asked us to wait (`retryAfterMs`, from Retry-After).
 */
export const recordTransientFailure = (tempId: string, error: string, now = Date.now(), retryAfterMs = 0): Promise<void> =>
  updateItem(tempId, (item) => {
    const attempts = item.attempts + 1;
    return {
      ...item,
      status: 'pending',
      attempts,
      lastError: error,
      nextAttemptAt: now + Math.max(backoffDelayMs(attempts), retryAfterMs),
    };
  });

/** The server rejected the report: keep it, with the reason, until the user acts. */
export const markReportFailed = (tempId: string, reason: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, status: 'failed', lastError: reason, nextAttemptAt: undefined }));

/** The user asked to send it again: pending, due now, backoff restarted. */
export const retryQueuedReport = (tempId: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, status: 'pending', attempts: 0, nextAttemptAt: undefined, lastError: undefined }));

/** The user corrected a report the server rejected. Saving queues it to be sent again. */
export const editQueuedReport = (tempId: string, changes: Partial<NewReport>): Promise<void> =>
  updateItem(tempId, (item) => ({
    ...item,
    ...changes,
    status: 'pending',
    attempts: 0,
    nextAttemptAt: undefined,
    lastError: undefined,
  }));

export const markReportCreated = (tempId: string, serverReportId: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, serverReportId }));

export const removeQueuedPhoto = (tempId: string, uri: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, photos: item.photos.filter((p) => p !== uri) }));
