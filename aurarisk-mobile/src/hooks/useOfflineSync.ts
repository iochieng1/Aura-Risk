import { useEffect, useState } from 'react';
import NetInfo from '@react-native-community/netinfo';
import { getReportQueue, dequeueReport, updateRetryCount } from '../storage/reportQueue';
import { submitReport } from '../services/api';

export const useOfflineSync = () => {
  const [isSyncing, setIsSyncing] = useState(false);
  const [pendingCount, setPendingCount] = useState(0);

  const syncPendingReports = async () => {
    const queue = await getReportQueue();
    setPendingCount(queue.length);

    if (queue.length === 0 || isSyncing) return;

    setIsSyncing(true);

    for (const report of queue) {
      if (report.retryCount >= 3) {
        await dequeueReport(report.tempId);
        continue;
      }

      try {
        const res = await submitReport(report, true);
        if (res.success) {
          await dequeueReport(report.tempId);
        } else {
          await updateRetryCount(report.tempId);
        }
      } catch {
        await updateRetryCount(report.tempId);
      }
    }

    const remainingQueue = await getReportQueue();
    setPendingCount(remainingQueue.length);
    setIsSyncing(false);
  };

  useEffect(() => {
    const unsubscribe = NetInfo.addEventListener((state) => {
      if (state.isConnected && state.isInternetReachable) {
        syncPendingReports();
      }
    });

    return () => unsubscribe();
  }, []);

  return { syncPendingReports, isSyncing, pendingCount };
};
