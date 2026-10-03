import { useCallback, useEffect, useRef, useState } from 'react';
import NetInfo from '@react-native-community/netinfo';
import { getReportQueue } from '../storage/reportQueue';
import { flushReportQueue } from '../sync/flushReportQueue';

export const useOfflineSync = () => {
  const [isSyncing, setIsSyncing] = useState(false);
  const [pendingCount, setPendingCount] = useState(0);
  // A ref, not state: the NetInfo listener is registered once and would otherwise see a stale value.
  const syncingRef = useRef(false);

  const syncPendingReports = useCallback(async () => {
    if (syncingRef.current) return;
    syncingRef.current = true;
    setIsSyncing(true);
    try {
      const { remaining } = await flushReportQueue();
      setPendingCount(remaining);
    } catch (err) {
      console.warn('Offline sync failed:', err);
      setPendingCount((await getReportQueue()).length);
    } finally {
      syncingRef.current = false;
      setIsSyncing(false);
    }
  }, []);

  const refreshPendingCount = useCallback(async () => {
    setPendingCount((await getReportQueue()).length);
  }, []);

  useEffect(() => {
    refreshPendingCount();
    const unsubscribe = NetInfo.addEventListener((state) => {
      if (state.isConnected && state.isInternetReachable !== false) {
        syncPendingReports();
      }
    });

    return () => unsubscribe();
  }, [syncPendingReports, refreshPendingCount]);

  return { syncPendingReports, refreshPendingCount, isSyncing, pendingCount };
};
