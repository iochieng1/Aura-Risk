import type { Location } from '@aurarisk/shared';
import { createContext, type ReactNode, useCallback, useContext, useMemo, useState } from 'react';
import { useOfflineSync } from '../hooks/useOfflineSync';

type OfflineSync = ReturnType<typeof useOfflineSync>;

interface AppState {
  /** The location every tab shows risk and reports for; null until the user picks one. */
  location: Location | null;
  setLocation: (location: Location) => void;
  /** Bumped when risk and reports should be fetched again, e.g. after a report is submitted. */
  refreshKey: number;
  /** Call after a report was submitted or queued. */
  onReportSubmitted: () => void;
  /** The report queue and its sync state, shared so one NetInfo listener drives every screen. */
  sync: OfflineSync;
  /** Bumped when the account is deleted, so screens holding account data (followed locations) reload. */
  accountGeneration: number;
  onAccountDeleted: () => void;
}

const AppStateContext = createContext<AppState | null>(null);

/** State shared by the tabs. Mount once, above the navigator. */
export function AppStateProvider({ children }: { children: ReactNode }) {
  const sync = useOfflineSync();
  const [location, setLocation] = useState<Location | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [accountGeneration, setAccountGeneration] = useState(0);
  const { refreshQueue } = sync;

  const onReportSubmitted = useCallback(() => {
    refreshQueue();
    setRefreshKey((k) => k + 1);
  }, [refreshQueue]);

  const onAccountDeleted = useCallback(() => setAccountGeneration((g) => g + 1), []);

  const value = useMemo<AppState>(
    () => ({ location, setLocation, refreshKey, onReportSubmitted, sync, accountGeneration, onAccountDeleted }),
    [location, refreshKey, onReportSubmitted, sync, accountGeneration, onAccountDeleted]
  );
  return <AppStateContext.Provider value={value}>{children}</AppStateContext.Provider>;
}

export function useAppState(): AppState {
  const state = useContext(AppStateContext);
  if (!state) throw new Error('useAppState must be used inside AppStateProvider');
  return state;
}
