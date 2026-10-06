import { MAX_LOCATION_NAME_BYTES, MAX_NOTE_BYTES, REPORT_CATEGORIES, RISK_LEVELS } from './constants';
import type {
  CommunityReport,
  Coordinates,
  Location,
  NewReportInput,
  ReportCategory,
  RiskAssessment,
  RiskLevel,
} from './types';

// Hand-written validators so the package has no runtime dependencies and works unchanged in
// Vite, Metro, and Jest. Rules mirror the Go handlers; the server stays the source of truth.

export type FieldErrors = Partial<Record<string, string>>;

export type ValidationResult<T> = { ok: true; value: T } | { ok: false; errors: FieldErrors };

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

const isFiniteNumber = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);

/** Length in UTF-8 bytes, which is what Go's len() measures on the server. */
export function utf8ByteLength(s: string): number {
  let bytes = 0;
  for (const ch of s) {
    const cp = ch.codePointAt(0)!;
    bytes += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
  }
  return bytes;
}

export function isReportCategory(v: unknown): v is ReportCategory {
  return REPORT_CATEGORIES.some((c) => c.value === v);
}

export function isRiskLevel(v: unknown): v is RiskLevel {
  return RISK_LEVELS.includes(v as RiskLevel);
}

export function validateCoordinates(lat: unknown, lon: unknown): ValidationResult<Coordinates> {
  const errors: FieldErrors = {};
  if (!isFiniteNumber(lat) || lat < -90 || lat > 90) errors.lat = 'Latitude must be between -90 and 90.';
  if (!isFiniteNumber(lon) || lon < -180 || lon > 180) errors.lon = 'Longitude must be between -180 and 180.';
  return Object.keys(errors).length ? { ok: false, errors } : { ok: true, value: { lat: lat as number, lon: lon as number } };
}

/** Parses coordinates typed by a person. Blank fields are errors rather than 0. */
export function parseCoordinates(latText: string, lonText: string): ValidationResult<Coordinates> {
  const toNumber = (s: string) => (s.trim() === '' ? NaN : Number(s.trim()));
  return validateCoordinates(toNumber(latText), toNumber(lonText));
}

export function validateLocation(input: unknown): ValidationResult<Location> {
  if (!isRecord(input)) return { ok: false, errors: { location: 'Choose a location.' } };
  const errors: FieldErrors = {};
  const name = typeof input.name === 'string' ? input.name.trim() : '';
  if (!name) errors.name = 'Location name is required.';
  else if (utf8ByteLength(name) > MAX_LOCATION_NAME_BYTES) errors.name = 'Location name is too long.';

  const coords = validateCoordinates(input.lat, input.lon);
  if (!coords.ok) Object.assign(errors, coords.errors);

  return Object.keys(errors).length || !coords.ok
    ? { ok: false, errors }
    : { ok: true, value: { name, lat: coords.value.lat, lon: coords.value.lon } };
}

/** Validates and normalises (trims) a report before it is sent or queued. */
export function validateNewReport(input: unknown): ValidationResult<NewReportInput> {
  if (!isRecord(input)) return { ok: false, errors: { report: 'Invalid report.' } };
  const errors: FieldErrors = {};

  const location = validateLocation(input.location);
  if (!location.ok) Object.assign(errors, location.errors);

  if (!isReportCategory(input.category)) errors.category = 'Choose a category.';

  const note = typeof input.note === 'string' ? input.note.trim() : '';
  if (!note) errors.note = 'Describe what you see.';
  else if (utf8ByteLength(note) > MAX_NOTE_BYTES) errors.note = 'Note is too long.';

  if (Object.keys(errors).length || !location.ok) return { ok: false, errors };
  return { ok: true, value: { location: location.value, category: input.category as ReportCategory, note } };
}

const isLocation = (v: unknown): v is Location =>
  isRecord(v) && typeof v.name === 'string' && isFiniteNumber(v.lat) && isFiniteNumber(v.lon);

export function isRiskAssessment(v: unknown): v is RiskAssessment {
  return (
    isRecord(v) &&
    isLocation(v.location) &&
    isFiniteNumber(v.score) &&
    isRiskLevel(v.level) &&
    typeof v.summary === 'string' &&
    Array.isArray(v.tips) &&
    v.tips.every((t) => typeof t === 'string')
  );
}

export function isCommunityReport(v: unknown): v is CommunityReport {
  return (
    isRecord(v) &&
    typeof v.id === 'string' &&
    isLocation(v.location) &&
    isReportCategory(v.category) &&
    typeof v.note === 'string' &&
    typeof v.timestamp === 'string' &&
    (v.photos === undefined || Array.isArray(v.photos))
  );
}
