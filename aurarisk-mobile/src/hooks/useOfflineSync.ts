import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import NetInfo from '@react-native-community/netinfo';
import { countQueue, getReportQueue, QueuedReport, subscribeToReportQueue } from '../storage/reportQueue';
import { flushReportQueue } from '../sync/flushReportQueue';

// While the app is open, check again for due reports no more often than this.
const MIN_RESCHEDULE_MS = 30_000;

export const useOfflineSync = () => {
  const [isSyncing, setIsSyncing] = useState(false);
  // The queue itself, kept current by the storage layer; every count shown is derived from it.
  const [queue, setQueue] = useState<QueuedReport[]>([]);
  // A ref, not state: the NetInfo listener is registered once and would otherwise see a stale value.
  const syncingRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const syncPendingReports = useCallback(async () => {
    if (syncingRef.current) return;
    syncingRef.current = true;
    setIsSyncing(true);
    clearTimeout(timerRef.current);
    try {
      const { nextAttemptAt } = await flushReportQueue();
      // Backoff only means something if someone looks again, so wake up when the next report is due.
      if (nextAttemptAt !== undefined) {
        const delay = Math.max(nextAttemptAt - Date.now(), MIN_RESCHEDULE_MS);
        timerRef.current = setTimeout(() => syncPendingReports(), delay);
      }
    } catch (err) {
      console.warn('Offline sync failed:', err);
    } finally {
      syncingRef.current = false;
      setIsSyncing(false);
      setQueue(await getReportQueue());
    }
  }, []);

  const refreshQueue = useCallback(async () => {
    setQueue(await getReportQueue());
  }, []);

  useEffect(() => {
    refreshQueue();
    const unsubscribeQueue = subscribeToReportQueue(setQueue);
    const unsubscribeNet = NetInfo.addEventListener((state) => {
      if (state.isConnected && state.isInternetReachable !== false) {
        syncPendingReports();
      }
    });

    return () => {
      unsubscribeQueue();
      unsubscribeNet();
      clearTimeout(timerRef.current);
    };
  }, [syncPendingReports, refreshQueue]);

  const { pending: pendingCount, failed: failedCount } = useMemo(() => countQueue(queue), [queue]);

  return { syncPendingReports, refreshQueue, isSyncing, queue, pendingCount, failedCount };
};
