import { describeFreshness, formatAge, type RiskAssessment } from '../index';

const NOW = Date.parse('2026-10-12T12:00:00Z');

const risk = (overrides: Partial<RiskAssessment> = {}): RiskAssessment => ({
  location: { name: 'Test', lat: 0, lon: 0 },
  score: 10,
  level: 'Normal',
  summary: '',
  tips: [],
  ...overrides,
});

describe('formatAge', () => {
  it.each([
    [NOW + 60_000, 'just now'],
    [NOW - 30_000, 'just now'],
    [NOW - 5 * 60_000, '5 min ago'],
    [NOW - 3 * 3_600_000, '3 h ago'],
    [NOW - 47 * 3_600_000, '47 h ago'],
    [NOW - 3 * 86_400_000, '3 days ago'],
  ])('%s -> %s', (from, want) => {
    expect(formatAge(from, NOW)).toBe(want);
  });
});

describe('describeFreshness', () => {
  it('returns null without timestamps from an older backend', () => {
    expect(describeFreshness(risk(), NOW)).toBeNull();
  });

  it('prefers the observation time over the fetch time', () => {
    const f = describeFreshness(
      risk({
        source_timestamps: { weather_fetched_at: '2026-10-12T11:58:00Z', weather_observed_at: '2026-10-12T11:45:00Z' },
      }),
      NOW
    );
    expect(f).toEqual({ label: 'Weather data from 15 min ago', outdated: false });
  });

  it('marks data older than two hours as outdated', () => {
    const f = describeFreshness(risk({ source_timestamps: { weather_fetched_at: '2026-10-12T09:00:00Z' } }), NOW);
    expect(f).toEqual({ label: 'Weather data from 3 h ago', outdated: true });
  });

  it('says when the backend served cached data', () => {
    const f = describeFreshness(
      risk({ stale: true, source_timestamps: { weather_fetched_at: '2026-10-12T11:30:00Z' } }),
      NOW
    );
    expect(f).toEqual({ label: 'Live weather is unavailable; showing data from 30 min ago', outdated: true });
    expect(describeFreshness(risk({ stale: true }), NOW)?.outdated).toBe(true);
  });

  it('ignores unparseable timestamps', () => {
    expect(describeFreshness(risk({ source_timestamps: { weather_fetched_at: 'nope' } }), NOW)).toBeNull();
  });
});
