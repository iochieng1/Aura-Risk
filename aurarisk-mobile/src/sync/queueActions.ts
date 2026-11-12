import { validateNewReport } from '@aurarisk/shared';
import { deleteLocalPhoto } from '../photos/photoStore';
import { ReportValidationError } from '../services/api';
import { discardQueuedReport, editQueuedReport, getReportQueue, retryQueuedReport } from '../storage/reportQueue';
import { ReportCategory } from '../types/api';

// What the user can do with a queued report. These are the only ways a report leaves the queue
// without being sent.

/** Makes the report due now and clears its backoff. The caller then flushes. */
export const retryReport = (tempId: string): Promise<void> => retryQueuedReport(tempId);

/** Saves the user's corrections to a rejected report and queues it again. Throws ReportValidationError. */
export const editReport = async (tempId: string, changes: { category: ReportCategory; note: string }): Promise<void> => {
  const item = (await getReportQueue()).find((q) => q.tempId === tempId);
  if (!item) return;
  const checked = validateNewReport({ ...item, ...changes });
  if (!checked.ok) throw new ReportValidationError(checked.errors);
  await editQueuedReport(tempId, { category: checked.value.category, note: checked.value.note });
};

/** Deletes the report and its local photos. Only ever called from an explicit user action. */
export const discardReport = async (tempId: string): Promise<void> => {
  const photos = await discardQueuedReport(tempId);
  photos.forEach((uri) => deleteLocalPhoto(uri));
};
