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

export const MAX_WARD_BYTES = 100;

/**
 * Kenya's 47 counties in official code order (1 Mombasa to 47 Nairobi).
 * Keep in sync with KenyaCounties in aurarisk-backend/internal/services/kenya.go.
 */
export const KENYA_COUNTIES = [
  'Mombasa', 'Kwale', 'Kilifi', 'Tana River', 'Lamu', 'Taita-Taveta', 'Garissa', 'Wajir', 'Mandera',
  'Marsabit', 'Isiolo', 'Meru', 'Tharaka-Nithi', 'Embu', 'Kitui', 'Machakos', 'Makueni', 'Nyandarua',
  'Nyeri', 'Kirinyaga', "Murang'a", 'Kiambu', 'Turkana', 'West Pokot', 'Samburu', 'Trans Nzoia',
  'Uasin Gishu', 'Elgeyo-Marakwet', 'Nandi', 'Baringo', 'Laikipia', 'Nakuru', 'Narok', 'Kajiado',
  'Kericho', 'Bomet', 'Kakamega', 'Vihiga', 'Bungoma', 'Busia', 'Siaya', 'Kisumu', 'Homa Bay',
  'Migori', 'Kisii', 'Nyamira', 'Nairobi',
] as const;

export type KenyaCounty = (typeof KENYA_COUNTIES)[number];

/** Counties sorted by name, for pickers. */
export const KENYA_COUNTIES_SORTED: readonly KenyaCounty[] = [...KENYA_COUNTIES].sort((a, b) => a.localeCompare(b));

export const DEFAULT_REPORT_RADIUS_KM = 10;

/**
 * Release gate (issue #25): every client shows this wherever a risk level is
 * shown, until an authorized agency designates AuraRisk an official warning service.
 */
export const DECISION_SUPPORT_NOTICE =
  'AuraRisk is a decision-support tool, not an official flood warning. Always follow instructions from local authorities and emergency services.';
