import { type Lang, translate } from './i18n';
import type { RiskAssessment } from './types';

/** Weather older than this is called out as outdated, matching the backend's confidence check. */
export const OUTDATED_WEATHER_MS = 2 * 60 * 60 * 1000;

export interface DataFreshness {
  /** e.g. "Weather data from 5 min ago". */
  label: string;
  /** True when the backend served cached data or the data is older than OUTDATED_WEATHER_MS. */
  outdated: boolean;
}

/** "just now", "5 min ago", "3 h ago", "2 days ago". Future times count as just now. */
export function formatAge(fromMs: number, nowMs: number, lang: Lang = 'en'): string {
  const minutes = Math.floor((nowMs - fromMs) / 60_000);
  if (minutes < 1) return translate(lang, 'age.justNow');
  if (minutes < 60) return translate(lang, 'age.minutes', { n: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return translate(lang, 'age.hours', { n: hours });
  return translate(lang, 'age.days', { n: Math.floor(hours / 24) });
}

/**
 * Describes how old an assessment's weather data is. Prefers when the
 * provider observed conditions over when the backend fetched them. Returns
 * null when an older backend sent no timestamps.
 */
export function describeFreshness(
  risk: RiskAssessment,
  nowMs: number = Date.now(),
  lang: Lang = 'en'
): DataFreshness | null {
  const ts = risk.source_timestamps;
  const source = ts?.weather_observed_at || ts?.weather_fetched_at;
  const at = source ? Date.parse(source) : NaN;
  if (!Number.isFinite(at)) {
    return risk.stale ? { label: translate(lang, 'freshness.stale'), outdated: true } : null;
  }
  const outdated = Boolean(risk.stale) || nowMs - at > OUTDATED_WEATHER_MS;
  const age = formatAge(at, nowMs, lang);
  return {
    label: translate(lang, risk.stale ? 'freshness.staleFrom' : 'freshness.from', { age }),
    outdated,
  };
}
