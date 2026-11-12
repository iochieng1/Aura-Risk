import { ApiError } from '@aurarisk/shared';
import { uploadReportPhotos } from '../photos/upload';
import { createReport, isOnline, ReportRejectedError } from '../services/api';
import {
  dequeueReport,
  getReportQueue,
  isDue,
  markReportCreated,
  markReportFailed,
  QueuedReport,
  recordTransientFailure,
  removeQueuedPhoto,
} from '../storage/reportQueue';

export interface FlushResult {
  sent: number;
  /** Reports the server rejected in this run; they stay queued as `failed`. */
  rejected: number;
  /** Reports that hit a transient error in this run; they stay pending with a later `nextAttemptAt`. */
  deferred: number;
  /** Reports still in the queue (pending + failed). */
  remaining: number;
  /** Earliest `nextAttemptAt` of a pending report, so the caller can schedule the next run. */
  nextAttemptAt?: number;
}

const flushOne = async (item: QueuedReport): Promise<void> => {
  let reportId = item.serverReportId;
  if (!reportId) {
    reportId = await createReport(item);
    // Persist immediately: if a photo fails below, the next retry must not create the report again.
    await markReportCreated(item.tempId, reportId);
  }
  await uploadReportPhotos(reportId, item.photos, (uri) => removeQueuedPhoto(item.tempId, uri));
  await dequeueReport(item.tempId);
};

const errorMessage = (err: unknown) => (err instanceof Error ? err.message : String(err));

/** Seconds from Retry-After on a 429/503, as ms. */
const retryAfterMs = (err: unknown) => (err instanceof ApiError && err.retryAfter ? err.retryAfter * 1000 : 0);

const runFlush = async (): Promise<FlushResult> => {
  const queue = await getReportQueue();
  let sent = 0;
  let rejected = 0;
  let deferred = 0;

  for (const item of queue) {
    if (!isDue(item)) continue;
    // Losing connectivity mid-flush isn't the report's fault, so stop without counting an attempt.
    if (!(await isOnline())) break;

    try {
      await flushOne(item);
      sent += 1;
    } catch (err) {
      if (err instanceof ReportRejectedError) {
        rejected += 1;
        await markReportFailed(item.tempId, err.message);
      } else {
        // Network errors, timeouts (fetchWithRetry never resends a POST that timed out), 5xx, 408,
        // 429 and photo upload failures: try again later.
        deferred += 1;
        await recordTransientFailure(item.tempId, errorMessage(err), Date.now(), retryAfterMs(err));
      }
    }
  }

  const after = await getReportQueue();
  const waits = after.filter((item) => item.status === 'pending').map((item) => item.nextAttemptAt ?? Date.now());
  return {
    sent,
    rejected,
    deferred,
    remaining: after.length,
    ...(waits.length > 0 ? { nextAttemptAt: Math.min(...waits) } : {}),
  };
};

let inFlight: Promise<FlushResult> | null = null;

/**
 * Sends queued reports that are due, with their photos. Never removes a report it couldn't send:
 * a server rejection marks it `failed` (kept for the user), anything else reschedules it with
 * backoff. The UI hook and the background task share one in-flight run, so the same item is never
 * submitted twice concurrently.
 */
export const flushReportQueue = (): Promise<FlushResult> => {
  if (!inFlight) {
    inFlight = runFlush().finally(() => {
      inFlight = null;
    });
  }
  return inFlight;
};
