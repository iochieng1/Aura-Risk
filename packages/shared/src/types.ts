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

export interface RiskAssessment {
  location: Location;
  score: number;
  level: RiskLevel;
  summary: string;
  tips: string[];
}

export type ReportCategory = 'flooding' | 'road_blocked' | 'water_rising' | 'drainage_issue';

export interface ReportPhoto {
  id: string;
  thumb_url: string;
  large_url: string;
}

export interface CommunityReport {
  id: string;
  location: Location;
  category: ReportCategory;
  note: string;
  /** RFC 3339 creation time. */
  timestamp: string;
  photos?: ReportPhoto[];
}

/** Body of POST /api/reports. */
export interface NewReportInput {
  location: Location;
  category: ReportCategory;
  note: string;
}

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
