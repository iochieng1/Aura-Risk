export interface RiskData {
  score: number;
  risk_level: string;
  model_version: string;
  confidence_score: number;
  source_timestamps: Record<string, string>;
  cachedAt?: number;
}

export interface CommunityReport {
  id?: string;
  tempId?: string;
  location: { lat: number; lon: number };
  category: 'flooding' | 'road_blocked' | 'water_rising' | 'drainage_issue';
  note: string;
  createdAt: string;
}

export interface CachedData<T> {
  timestamp: number;
  data: T;
}