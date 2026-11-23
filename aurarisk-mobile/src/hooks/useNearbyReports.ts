import type { CommunityReport, Location } from '@aurarisk/shared';
import { useEffect, useState } from 'react';
import { fetchNearbyReports } from '../services/api';

interface NearbyReports {
  reports: CommunityReport[];
  /** True when the reports came from this device's cache because the server could not be reached. */
  isOffline: boolean;
  loading: boolean;
}

/** Community reports near `location`, from the server or, offline, from the cache. */
export function useNearbyReports(location: Location | null, refreshKey = 0): NearbyReports {
  const [state, setState] = useState<NearbyReports>({ reports: [], isOffline: false, loading: false });

  useEffect(() => {
    if (!location) {
      setState({ reports: [], isOffline: false, loading: false });
      return;
    }
    let cancelled = false;
    setState((s) => ({ ...s, loading: true }));
    fetchNearbyReports(location.lat, location.lon).then(({ reports, isOffline }) => {
      if (!cancelled) setState({ reports, isOffline, loading: false });
    });
    return () => {
      cancelled = true;
    };
  }, [location?.lat, location?.lon, refreshKey]);

  return state;
}
