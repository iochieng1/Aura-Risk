import type { Location, RiskAssessment, CommunityReport, ReportCategory } from "../types";
import { createApiClient } from "@aurarisk/shared";

// Empty means same origin: the Vite dev server proxies /api to the backend,
// and in production a reverse proxy can do the same.
const api = createApiClient({ baseUrl: import.meta.env.VITE_API_BASE_URL ?? "" });

// Mock data is for working on the UI without a backend. import.meta.env.DEV is
// false in production builds, so Vite removes these branches and mock.ts.
const USE_MOCK = import.meta.env.DEV && import.meta.env.VITE_USE_MOCK_API === "true";
const loadMock = () => import("../utils/mock");

export async function fetchRisk(location: Location): Promise<RiskAssessment> {
  if (USE_MOCK) return (await loadMock()).getMockRisk(location);
  return api.getRisk(location);
}

export async function fetchReports(location: Location): Promise<CommunityReport[]> {
  if (USE_MOCK) return (await loadMock()).getMockReports();
  return api.getReports(location, 10);
}

export async function submitReport(
  location: Location,
  category: ReportCategory,
  note: string
): Promise<CommunityReport> {
  if (USE_MOCK) return (await loadMock()).createMockReport({ location, category, note });
  return api.createReport({ location, category, note });
}
