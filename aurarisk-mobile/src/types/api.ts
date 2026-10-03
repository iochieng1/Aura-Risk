export interface RiskData {
  score: number;
  risk_level: string;
  model_version: string;
  confidence_score: number;
  source_timestamps: Record<string, string>;
  cachedAt?: number;
}

export type ReportCategory = 'flooding' | 'road_blocked' | 'water_rising' | 'drainage_issue';

export interface CommunityReport {
  id?: string;
  tempId?: string;
  location: { name: string; lat: number; lon: number };
  category: ReportCategory;
  note: string;
  createdAt: string;
  photos?: ReportPhoto[];
}

export interface ReportPhoto {
  id: string;
  thumb_url: string;
  large_url: string;
}

export interface CachedData<T> {
  timestamp: number;
  data: T;
}

export interface TokenPair {
  account_id: string;
  device_id: string;
  access_token: string;
  access_expires_at: string;
  refresh_token: string;
  refresh_expires_at: string;
}

export type RiskLevel = 'Advisory' | 'Alert' | 'Emergency';

export interface QuietHours {
  start: string; // "HH:MM"
  end: string; // "HH:MM"
  timezone: string; // IANA
}

export interface SubscriptionInput {
  name: string;
  lat: number;
  lon: number;
  min_level: RiskLevel;
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
