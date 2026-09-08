export type RiskLevel = "Normal" | "Advisory" | "Alert" | "Emergency";

export interface Location {
  name: string;
  lat: number;
  lon: number;
}

export interface RiskAssessment {
  location: Location;
  score: number;
  level: RiskLevel;
  summary: string;
  tips: string[];
}

export type ReportCategory =
  | "flooding"
  | "road_blocked"
  | "water_rising"
  | "drainage_issue";

export interface CommunityReport {
  id: string;
  location: Location;
  category: ReportCategory;
  note: string;
  timestamp: string;
}
