import { uploadReportPhotos } from '../photos/upload';
import { deleteLocalPhoto } from '../photos/photoStore';
import { createReport, isOnline, ReportRejectedError } from '../services/api';
import {
  dequeueReport,
  getReportQueue,
  markReportCreated,
  QueuedReport,
  removeQueuedPhoto,
  updateRetryCount,
} from '../storage/reportQueue';

export const MAX_RETRIES = 3;

export interface FlushResult {
  sent: number;
  failed: number;
  remaining: number;
}

const dropItem = async (item: QueuedReport) => {
  item.photos.forEach((uri) => deleteLocalPhoto(uri));
  await dequeueReport(item.tempId);
};

const flushOne = async (item: QueuedReport): Promise<boolean> => {
  let reportId = item.serverReportId;
  if (!reportId) {
    reportId = await createReport(item);
    // Persist immediately: if a photo fails below, the next retry must not create the report again.
    await markReportCreated(item.tempId, reportId);
  }
  await uploadReportPhotos(reportId, item.photos, (uri) => removeQueuedPhoto(item.tempId, uri));
  await dequeueReport(item.tempId);
  return true;
};

const runFlush = async (): Promise<FlushResult> => {
  const queue = await getReportQueue();
  let sent = 0;
  let failed = 0;

  for (const item of queue) {
    if (item.retryCount >= MAX_RETRIES) {
      await dropItem(item);
      continue;
    }
    // Losing connectivity mid-flush isn't the report's fault, so stop without counting a retry.
    if (!(await isOnline())) break;

    try {
      await flushOne(item);
      sent += 1;
    } catch (err) {
      failed += 1;
      if (err instanceof ReportRejectedError) {
        await dropItem(item);
      } else {
        await updateRetryCount(item.tempId);
      }
    }
  }

  const remaining = (await getReportQueue()).length;
  return { sent, failed, remaining };
};

let inFlight: Promise<FlushResult> | null = null;

/**
 * Sends queued reports and their photos. The UI hook and the background task share one in-flight
 * run, so the same item is never submitted twice concurrently.
 */
export const flushReportQueue = (): Promise<FlushResult> => {
  if (!inFlight) {
    inFlight = runFlush().finally(() => {
      inFlight = null;
    });
  }
  return inFlight;
};
