import type { Location, RiskAssessment, CommunityReport, ReportCategory } from "../types";
import { getMockRisk, getMockReports } from "../utils/mock";

const USE_BACKEND = false;
const BASE = "/api";

export async function fetchRisk(location: Location): Promise<RiskAssessment> {
  if (!USE_BACKEND) return getMockRisk(location);

  const res = await fetch(`${BASE}/risk?lat=${location.lat}&lon=${location.lon}`);
  if (!res.ok) throw new Error("Failed to fetch risk");
  return res.json();
}

export async function fetchReports(location: Location): Promise<CommunityReport[]> {
  if (!USE_BACKEND) return getMockReports();

  const res = await fetch(
    `${BASE}/reports?lat=${location.lat}&lon=${location.lon}&radius=10`
  );
  if (!res.ok) throw new Error("Failed to fetch reports");
  return res.json();
}

export async function submitReport(
  location: Location,
  category: ReportCategory,
  note: string
): Promise<CommunityReport> {
  const report: CommunityReport = {
    id: crypto.randomUUID(),
    location,
    category,
    note,
    timestamp: new Date().toISOString(),
  };

  if (!USE_BACKEND) return report;

  const res = await fetch(`${BASE}/reports`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ location, category, note }),
  });
  if (!res.ok) throw new Error("Failed to submit report");
  return res.json();
}
