import type { KenyaCounty } from './constants';

// Wire types for the AuraRisk HTTP API. Field names match the JSON the Go backend emits
// (aurarisk-backend/internal/models). Change them together.

export type RiskLevel = 'Normal' | 'Advisory' | 'Alert' | 'Emergency';

/** Levels a subscription can be notified at (Normal never triggers an alert). */
export type AlertLevel = Exclude<RiskLevel, 'Normal'>;

export interface Coordinates {
  lat: number;
  lon: number;
}

export interface Location extends Coordinates {
  name: string;
}

export type ConfidenceLevel = 'low' | 'medium' | 'high';

export interface RiskConfidence {
  /** Trust in the inputs and model, not a probability that the score is right. */
  level: ConfidenceLevel;
  /** Plain-language reasons the level is not higher. */
  reasons: string[];
}

export interface RiskSourceTimestamps {
  /** ISO 8601: when the backend retrieved the weather data. */
  weather_fetched_at: string;
  /** ISO 8601: when the provider's current conditions were valid. */
  weather_observed_at?: string;
}

/** Each field is absent when its input was unavailable. */
export interface RiskHydrology {
  /** Ground elevation, in metres. */
  elevation_m?: number;
  /** How far the point lies below its ~1 km surroundings, in metres. */
  depression_m?: number;
  /** Nearby main river: median discharge over the past 92 days, in m³/s. */
  river_baseline_m3s?: number;
  /** Highest daily discharge forecast for the next 3 days, in m³/s. */
  river_peak_m3s?: number;
}

export interface RiskAssessment {
  location: Location;
  score: number;
  level: RiskLevel;
  summary: string;
  tips: string[];
  /** True when live weather was unavailable and cached data was used. Absent from older backends. */
  stale?: boolean;
  source_timestamps?: RiskSourceTimestamps;
  /** Ground and river data behind the score, when available. */
  hydrology?: RiskHydrology;
  confidence?: RiskConfidence;
  /** Scoring logic that produced the score, e.g. "heuristic-2". */
  model_version?: string;
}

export type ReportCategory = 'flooding' | 'road_blocked' | 'water_rising' | 'drainage_issue';

export interface ReportPhoto {
  id: string;
  thumb_url: string;
  large_url: string;
}

/** Moderation status. Public listings show pending and verified reports. */
export type ModerationStatus = 'pending' | 'verified' | 'rejected';

/** County response, set by the county's responders once a report is verified. */
export type ResponseStatus = 'open' | 'acknowledged' | 'resolved';

export interface CommunityReport {
  id: string;
  location: Location;
  category: ReportCategory;
  note: string;
  /** RFC 3339 creation time. */
  timestamp: string;
  photos?: ReportPhoto[];
  /** Absent from older servers; treat as pending (unverified). */
  status?: ModerationStatus;
  /** Independent nearby reports of the same kind of problem. */
  corroborations?: number;
  /**
   * Only on POST /api/reports: the earlier report this one repeats. Duplicates
   * are hidden from listings; show the original instead.
   */
  duplicate_of?: string;
  /** Kenyan county (official name) and ward, when the reporter gave them. */
  county?: string;
  ward?: string;
  /** Absent from older servers; treat as open. */
  response_status?: ResponseStatus;
  /** RFC 3339: when the response status last changed. Absent while open. */
  response_updated_at?: string;
}

/** Body of POST /api/reports. */
export interface NewReportInput {
  location: Location;
  category: ReportCategory;
  note: string;
  /** Optional Kenyan county; a ward needs one. */
  county?: KenyaCounty;
  ward?: string;
}

/** What a reporter fills in; the location comes from the map. */
export type NewReportDetails = Omit<NewReportInput, 'location'>;

export interface TokenPair {
  account_id: string;
  device_id: string;
  access_token: string;
  access_expires_at: string;
  refresh_token: string;
  refresh_expires_at: string;
}

export interface QuietHours {
  start: string; // "HH:MM"
  end: string; // "HH:MM"
  timezone: string; // IANA
}

export interface SubscriptionInput {
  name: string;
  lat: number;
  lon: number;
  min_level: AlertLevel;
  quiet_hours: QuietHours | null;
  allow_emergency_during_quiet_hours: boolean;
}

export interface Subscription extends SubscriptionInput {
  id: string;
  last_notified_level: string | null;
  last_notified_at: string | null;
  created_at: string;
}

export interface PhotoUpload {
  photo_id: string;
  upload_url: string;
  upload_method: 'PUT';
  upload_headers: Record<string, string>;
  expires_at: string;
}

export interface Photo {
  id: string;
  status: 'pending_upload' | 'processing' | 'ready' | 'rejected' | 'failed';
  thumb_url?: string;
  large_url?: string;
}

export interface RiskAlertData {
  type: 'risk_alert';
  subscription_id: string;
  level: string;
  score: number;
  lat: number;
  lon: number;
}
