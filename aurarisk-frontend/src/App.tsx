import { useState, useCallback, useEffect, useRef } from "react";
import { DECISION_SUPPORT_NOTICE } from "@aurarisk/shared";
import type { Location, RiskAssessment, CommunityReport, NewReportDetails } from "./types";
import { fetchRisk, fetchReports, submitReport } from "./api/risk";
import { describeError } from "./api/errors";
import Header from "./components/Header";
import Sidebar from "./components/Sidebar";
import MapView from "./components/MapView";
import MapListAlternative, { MAP_LIST_ID } from "./components/MapListAlternative";
import CountyDashboard from "./components/CountyDashboard";
import { useOnlineStatus } from "./hooks/useOnlineStatus";
import { useHashView } from "./hooks/useHashView";

// Kisumu, Kenya: the first pilot area (#56). VITE_DEFAULT_CENTER ("lat,lon")
// moves the map elsewhere without a code change.
const KISUMU: [number, number] = [34.768, -0.0917];
const DEFAULT_CENTER = parseCenter(import.meta.env.VITE_DEFAULT_CENTER) ?? KISUMU;

/** Parses "lat,lon" into the map's [lon, lat] order. */
function parseCenter(value: string | undefined): [number, number] | null {
  const parts = (value ?? "").split(",").map((part) => part.trim());
  if (parts.length !== 2 || parts.some((part) => part === "")) return null;
  const [lat, lon] = parts.map(Number);
  if (!Number.isFinite(lat) || !Number.isFinite(lon) || Math.abs(lat) > 90 || Math.abs(lon) > 180) return null;
  return [lon, lat];
}

export default function App() {
  const [center, setCenter] = useState<[number, number]>(DEFAULT_CENTER);
  const [selectedLocation, setSelectedLocation] = useState<Location | null>(null);
  const [risk, setRisk] = useState<RiskAssessment | null>(null);
  const [reports, setReports] = useState<CommunityReport[]>([]);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reportsFailed, setReportsFailed] = useState(false);
  const [locating, setLocating] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const online = useOnlineStatus();
  const view = useHashView();
  const menuButton = useRef<HTMLButtonElement>(null);
  // Only the latest selection may update state; slower earlier requests are ignored.
  const latestRequest = useRef(0);

  const loadLocation = useCallback(async (loc: Location) => {
    const request = ++latestRequest.current;
    setSelectedLocation(loc);
    setCenter([loc.lon, loc.lat]);
    setRisk(null);
    setReports([]);
    setLoadError(null);
    setReportsFailed(false);
    setNotice(null);
    setLoading(true);

    // Load independently so a weather outage doesn't hide community reports.
    const [riskResult, reportsResult] = await Promise.allSettled([
      fetchRisk(loc),
      fetchReports(loc),
    ]);
    if (request !== latestRequest.current) return;

    if (riskResult.status === "fulfilled") setRisk(riskResult.value);
    else console.error("Failed to load risk", riskResult.reason);
    if (reportsResult.status === "fulfilled") setReports(reportsResult.value);
    else console.error("Failed to load reports", reportsResult.reason);
    setReportsFailed(reportsResult.status === "rejected");

    const riskError = riskResult.status === "rejected" ? describeError(riskResult.reason) : null;
    const reportsError = reportsResult.status === "rejected" ? describeError(reportsResult.reason) : null;
    if (riskError && reportsError && riskError === reportsError) {
      setLoadError(`Couldn't load flood risk or reports. ${riskError}`);
    } else {
      setLoadError(
        [
          riskError && `Flood risk is unavailable. ${riskError}`,
          reportsError && `Community reports are unavailable. ${reportsError}`,
        ]
          .filter(Boolean)
          .join(" ") || null
      );
    }
    setLoading(false);
  }, []);

  const retry = useCallback(() => {
    if (selectedLocation) loadLocation(selectedLocation);
  }, [selectedLocation, loadLocation]);

  // Reload a failed location once the connection comes back.
  const wasOnline = useRef(online);
  useEffect(() => {
    if (online && !wasOnline.current && loadError) retry();
    wasOnline.current = online;
  }, [online, loadError, retry]);

  const handleLocate = useCallback(() => {
    if (!navigator.geolocation) {
      setNotice("This browser can't share your location. Search for a place instead.");
      setSidebarOpen(true);
      return;
    }
    setLocating(true);
    setNotice(null);
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        setLocating(false);
        const loc: Location = {
          name: "My Location",
          lat: pos.coords.latitude,
          lon: pos.coords.longitude,
        };
        loadLocation(loc);
      },
      (err) => {
        console.warn("Geolocation error", err);
        setLocating(false);
        setNotice(
          err.code === err.PERMISSION_DENIED
            ? "Location access is blocked. Allow it in your browser settings, or search for a place instead."
            : "Couldn't find your location. Try again, or search for a place instead."
        );
        // On small screens the message is in the drawer, so open it.
        setSidebarOpen(true);
      },
      { timeout: 15_000, maximumAge: 60_000 }
    );
  }, [loadLocation]);

  const closeSidebar = useCallback(() => {
    setSidebarOpen(false);
    menuButton.current?.focus();
  }, []);

  // Throws a user-facing message so the form can keep the note and show it.
  const handleSubmitReport = useCallback(
    async (details: NewReportDetails) => {
      if (!selectedLocation) throw new Error("Choose a location first.");
      let newReport: CommunityReport;
      try {
        newReport = await submitReport(selectedLocation, details);
      } catch (err) {
        console.error("Failed to submit report", err);
        throw new Error(describeError(err));
      }
      // A duplicate is stored but not listed, so don't show it twice.
      if (!newReport.duplicate_of) setReports((prev) => [newReport, ...prev]);
      return newReport;
    },
    [selectedLocation]
  );

  return (
    <div className="h-full flex flex-col">
      {view === "map" && (
        <a
          href={`#${MAP_LIST_ID}`}
          className="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:m-2 focus:px-3 focus:py-2 focus:bg-white focus:text-blue-700 focus:rounded-lg focus:shadow"
        >
          Skip to map contents as a list
        </a>
      )}
      <Header
        ref={menuButton}
        view={view}
        onLocate={handleLocate}
        locating={locating}
        onToggleSidebar={() => setSidebarOpen((o) => !o)}
        sidebarOpen={sidebarOpen}
      />
      {view === "counties" ? (
        <CountyDashboard />
      ) : (
        <div className="flex-1 flex flex-col md:flex-row overflow-hidden">
          <Sidebar
            onSelectLocation={loadLocation}
            risk={risk}
            reports={reports}
            selectedLocation={selectedLocation}
            loading={loading}
            loadError={loadError}
            reportsFailed={reportsFailed}
            onRetry={retry}
            online={online}
            notice={notice}
            onSubmitReport={handleSubmitReport}
            open={sidebarOpen}
            onClose={closeSidebar}
          />
          <main className="flex-1 relative">
            <MapListAlternative location={selectedLocation} risk={risk} reports={reports} />
            <MapView center={center} risk={risk} reports={reports} />
          </main>
        </div>
      )}
      <footer className="px-4 py-2 text-[11px] leading-snug text-gray-700 bg-amber-50 border-t border-amber-200">
        <strong>Decision support only.</strong> {DECISION_SUPPORT_NOTICE}
      </footer>
    </div>
  );
}
