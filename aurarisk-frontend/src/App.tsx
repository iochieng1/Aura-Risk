import { useState, useCallback } from "react";
import type { Location, RiskAssessment, CommunityReport, ReportCategory } from "./types";
import { fetchRisk, fetchReports, submitReport } from "./api/risk";
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

  const loadLocation = useCallback(async (loc: Location) => {
    setSelectedLocation(loc);
    setCenter([loc.lon, loc.lat]);

    try {
      const [riskData, reportData] = await Promise.all([
        fetchRisk(loc),
        fetchReports(loc),
      ]);
      setRisk(riskData);
      setReports(reportData);
    } catch (err) {
      console.error("Failed to load location data", err);
    }
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

  const handleSubmitReport = useCallback(
    async (category: ReportCategory, note: string) => {
      if (!selectedLocation) return;
      try {
        const newReport = await submitReport(selectedLocation, category, note);
        setReports((prev) => [newReport, ...prev]);
      } catch (err) {
        console.error("Failed to submit report", err);
      }
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
