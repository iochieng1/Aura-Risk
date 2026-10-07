import { useState, useCallback, useRef } from "react";
import type { Location, RiskAssessment, CommunityReport, ReportCategory } from "./types";
import { fetchRisk, fetchReports, submitReport } from "./api/risk";
import { describeError } from "./api/errors";
import Header from "./components/Header";
import Sidebar from "./components/Sidebar";
import MapView from "./components/MapView";

const DEFAULT_CENTER: [number, number] = [120.9842, 14.5995];

export default function App() {
  const [center, setCenter] = useState<[number, number]>(DEFAULT_CENTER);
  const [selectedLocation, setSelectedLocation] = useState<Location | null>(null);
  const [risk, setRisk] = useState<RiskAssessment | null>(null);
  const [reports, setReports] = useState<CommunityReport[]>([]);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  // Only the latest selection may update state; slower earlier requests are ignored.
  const latestRequest = useRef(0);

  const loadLocation = useCallback(async (loc: Location) => {
    const request = ++latestRequest.current;
    setSelectedLocation(loc);
    setCenter([loc.lon, loc.lat]);
    setRisk(null);
    setReports([]);
    setLoadError(null);
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

  const handleLocate = useCallback(() => {
    if (!navigator.geolocation) return;
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        const loc: Location = {
          name: "My Location",
          lat: pos.coords.latitude,
          lon: pos.coords.longitude,
        };
        loadLocation(loc);
      },
      (err) => console.warn("Geolocation error", err)
    );
  }, [loadLocation]);

  // Throws a user-facing message so the form can keep the note and show it.
  const handleSubmitReport = useCallback(
    async (category: ReportCategory, note: string) => {
      if (!selectedLocation) throw new Error("Choose a location first.");
      let newReport: CommunityReport;
      try {
        newReport = await submitReport(selectedLocation, category, note);
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
      <Header
        onLocate={handleLocate}
        onToggleSidebar={() => setSidebarOpen((o) => !o)}
        sidebarOpen={sidebarOpen}
      />
      <div className="flex-1 flex flex-col md:flex-row overflow-hidden">
        <Sidebar
          onSelectLocation={loadLocation}
          risk={risk}
          reports={reports}
          selectedLocation={selectedLocation}
          loading={loading}
          loadError={loadError}
          onSubmitReport={handleSubmitReport}
          open={sidebarOpen}
          onClose={() => setSidebarOpen(false)}
        />
        <main className="flex-1 relative">
          <MapView center={center} risk={risk} reports={reports} />
        </main>
      </div>
    </div>
  );
}
