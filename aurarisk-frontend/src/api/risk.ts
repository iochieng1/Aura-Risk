import type { Location, RiskAssessment, CommunityReport, ReportCategory } from "../types";
import { createApiClient, validateNewReport, ValidationError } from "@aurarisk/shared";
import { getMockRisk, getMockReports } from "../utils/mock";

const USE_BACKEND = false;

// Same-origin: Vite proxies /api to the backend in development.
const api = createApiClient({ baseUrl: "" });

export async function fetchRisk(location: Location): Promise<RiskAssessment> {
  if (!USE_BACKEND) return getMockRisk(location);
  return api.getRisk(location);
}

export async function fetchReports(location: Location): Promise<CommunityReport[]> {
  if (!USE_BACKEND) return getMockReports();
  return api.getReports(location, 10);
}

export async function submitReport(
  location: Location,
  category: ReportCategory,
  note: string
): Promise<CommunityReport> {
  if (USE_BACKEND) return api.createReport({ location, category, note });

  const checked = validateNewReport({ location, category, note });
  if (!checked.ok) throw new ValidationError(checked.errors);
  return {
    id: crypto.randomUUID(),
    ...checked.value,
    timestamp: new Date().toISOString(),
  };
}
