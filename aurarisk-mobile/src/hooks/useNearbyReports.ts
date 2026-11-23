import type { CommunityReport, Location } from '@aurarisk/shared';
import { useEffect, useRef, useState } from 'react';
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
  const shownFor = useRef<string | null>(null);

  useEffect(() => {
    if (!location) {
      shownFor.current = null;
      setState({ reports: [], isOffline: false, loading: false });
      return;
    }
    let cancelled = false;
    // A new location drops the previous one's reports so they are never shown as belonging to it.
    // A refresh of the same location keeps them on screen until the new ones arrive.
    const key = `${location.lat},${location.lon}`;
    const sameLocation = shownFor.current === key;
    shownFor.current = key;
    setState((s) => (sameLocation ? { ...s, loading: true } : { reports: [], isOffline: false, loading: true }));
    fetchNearbyReports(location.lat, location.lon)
      .then(({ reports, isOffline }) => {
        if (!cancelled) setState({ reports, isOffline, loading: false });
      })
      .catch((err) => {
        console.warn('Loading nearby reports failed:', err);
        if (!cancelled) setState({ reports: [], isOffline: true, loading: false });
      });
    return () => {
      cancelled = true;
    };
  }, [location?.lat, location?.lon, refreshKey]);

  return state;
}
