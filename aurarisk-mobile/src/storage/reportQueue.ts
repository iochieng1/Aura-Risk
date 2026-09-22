import AsyncStorage from '@react-native-async-storage/async-storage';
import { CommunityReport } from '../types/api';

const QUEUE_KEY = '@aurarisk/offline_reports_queue';

export interface QueuedReport extends CommunityReport {
  tempId: string;
  retryCount: number;
  createdAt: string;
}

export const getReportQueue = async (): Promise<QueuedReport[]> => {
  try {
    const raw = await AsyncStorage.getItem(QUEUE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
};

export const enqueueReport = async (report: Omit<CommunityReport, 'createdAt'>): Promise<QueuedReport> => {
  const queuedItem: QueuedReport = {
    ...report,
    tempId: `pending_${Date.now()}_${Math.random().toString(36).substring(2, 7)}`,
    retryCount: 0,
    createdAt: new Date().toISOString(),
  };

  const currentQueue = await getReportQueue();
  currentQueue.push(queuedItem);
  await AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(currentQueue));

  return queuedItem;
};

export const dequeueReport = async (tempId: string): Promise<void> => {
  const currentQueue = await getReportQueue();
  const filtered = currentQueue.filter((item) => item.tempId !== tempId);
  await AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(filtered));
};

export const updateRetryCount = async (tempId: string): Promise<void> => {
  const currentQueue = await getReportQueue();
  const updated = currentQueue.map((item) => 
    item.tempId === tempId ? { ...item, retryCount: item.retryCount + 1 } : item
  );
  await AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(updated));
};