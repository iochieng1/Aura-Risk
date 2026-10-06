import type { ReportCategory, RiskLevel } from './types';

export const RISK_LEVELS: readonly RiskLevel[] = ['Normal', 'Advisory', 'Alert', 'Emergency'];

/** Platform-neutral colour for each level; each app layers its own styling on top. */
export const RISK_LEVEL_COLORS: Record<RiskLevel, string> = {
  Normal: '#22c55e',
  Advisory: '#eab308',
  Alert: '#f97316',
  Emergency: '#ef4444',
};

export function scoreToLevel(score: number): RiskLevel {
  if (score >= 75) return 'Emergency';
  if (score >= 50) return 'Alert';
  if (score >= 25) return 'Advisory';
  return 'Normal';
}

export const REPORT_CATEGORIES: readonly { value: ReportCategory; label: string }[] = [
  { value: 'flooding', label: 'Flooding' },
  { value: 'road_blocked', label: 'Road blocked' },
  { value: 'water_rising', label: 'Water rising' },
  { value: 'drainage_issue', label: 'Drainage issue' },
];

// Limits enforced by POST /api/reports (handlers/reports.go). The server counts bytes.
export const MAX_LOCATION_NAME_BYTES = 200;
export const MAX_NOTE_BYTES = 2000;

export const DEFAULT_REPORT_RADIUS_KM = 10;
