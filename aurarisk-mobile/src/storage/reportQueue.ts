import AsyncStorage from '@react-native-async-storage/async-storage';
import { NewReportInput } from '../types/api';

const QUEUE_KEY = '@aurarisk/offline_reports_queue';

export interface QueuedReport extends NewReportInput {
  tempId: string;
  retryCount: number;
  createdAt: string;
  /** Local file URIs (app document directory) still waiting to be uploaded. */
  photos: string[];
  /** Set once the report exists on the server, so retries only upload the remaining photos. */
  serverReportId?: string;
}

export type NewReport = NewReportInput;

const writeQueue = (queue: QueuedReport[]) => AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(queue));

export const getReportQueue = async (): Promise<QueuedReport[]> => {
  try {
    const raw = await AsyncStorage.getItem(QUEUE_KEY);
    const parsed: QueuedReport[] = raw ? JSON.parse(raw) : [];
    // Items queued before photo support have no `photos` field.
    return parsed.map((item) => ({ ...item, photos: Array.isArray(item.photos) ? item.photos : [] }));
  } catch {
    return [];
  }
};

const updateItem = async (tempId: string, update: (item: QueuedReport) => QueuedReport): Promise<void> => {
  const currentQueue = await getReportQueue();
  await writeQueue(currentQueue.map((item) => (item.tempId === tempId ? update(item) : item)));
};

export const enqueueReport = async (
  report: NewReport,
  photos: string[] = [],
  serverReportId?: string
): Promise<QueuedReport> => {
  const queuedItem: QueuedReport = {
    ...report,
    tempId: `pending_${Date.now()}_${Math.random().toString(36).substring(2, 7)}`,
    retryCount: 0,
    createdAt: new Date().toISOString(),
    photos,
    ...(serverReportId ? { serverReportId } : {}),
  };

  const currentQueue = await getReportQueue();
  currentQueue.push(queuedItem);
  await writeQueue(currentQueue);

  return queuedItem;
};

export const dequeueReport = async (tempId: string): Promise<void> => {
  const currentQueue = await getReportQueue();
  await writeQueue(currentQueue.filter((item) => item.tempId !== tempId));
};

export const updateRetryCount = (tempId: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, retryCount: item.retryCount + 1 }));

export const markReportCreated = (tempId: string, serverReportId: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, serverReportId }));

export const removeQueuedPhoto = (tempId: string, uri: string): Promise<void> =>
  updateItem(tempId, (item) => ({ ...item, photos: item.photos.filter((p) => p !== uri) }));
