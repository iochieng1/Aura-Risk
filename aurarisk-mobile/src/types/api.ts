// API types are shared with the web app; see packages/shared. Only mobile-local types live here.
import type { RiskAssessment } from '@aurarisk/shared';

export type {
  AlertLevel,
  CommunityReport,
  Location,
  NewReportInput,
  Photo,
  PhotoUpload,
  QuietHours,
  ReportCategory,
  ReportPhoto,
  RiskAlertData,
  Subscription,
  SubscriptionInput,
  TokenPair,
} from '@aurarisk/shared';

/** Subscriptions can only be notified at these levels. */
export type { AlertLevel as RiskLevel } from '@aurarisk/shared';

export type RiskData = RiskAssessment & { cachedAt?: number };

export interface CachedData<T> {
  timestamp: number;
  data: T;
}
