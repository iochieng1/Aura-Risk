import { isValidTime, isWithinQuietHours, minutesInTimeZone } from '../quietHours';

const at = (iso: string) => new Date(iso);

describe('isWithinQuietHours', () => {
  const overnightUtc = { start: '22:00', end: '07:00', timezone: 'UTC' };

  it('returns false without quiet hours', () => {
    expect(isWithinQuietHours(null, at('2026-10-03T23:00:00Z'))).toBe(false);
  });

  it('handles overnight windows on both sides of midnight', () => {
    expect(isWithinQuietHours(overnightUtc, at('2026-10-03T22:00:00Z'))).toBe(true); // start inclusive
    expect(isWithinQuietHours(overnightUtc, at('2026-10-03T23:59:00Z'))).toBe(true);
    expect(isWithinQuietHours(overnightUtc, at('2026-10-04T00:00:00Z'))).toBe(true);
    expect(isWithinQuietHours(overnightUtc, at('2026-10-04T06:59:00Z'))).toBe(true);
    expect(isWithinQuietHours(overnightUtc, at('2026-10-04T07:00:00Z'))).toBe(false); // end exclusive
    expect(isWithinQuietHours(overnightUtc, at('2026-10-04T12:00:00Z'))).toBe(false);
    expect(isWithinQuietHours(overnightUtc, at('2026-10-04T21:59:00Z'))).toBe(false);
  });

  it('handles same-day windows', () => {
    const nap = { start: '13:00', end: '15:30', timezone: 'UTC' };
    expect(isWithinQuietHours(nap, at('2026-10-03T12:59:00Z'))).toBe(false);
    expect(isWithinQuietHours(nap, at('2026-10-03T13:00:00Z'))).toBe(true);
    expect(isWithinQuietHours(nap, at('2026-10-03T15:29:00Z'))).toBe(true);
    expect(isWithinQuietHours(nap, at('2026-10-03T15:30:00Z'))).toBe(false);
  });

  it('evaluates the window in the subscription time zone', () => {
    // Nairobi is UTC+3: 20:00 UTC is 23:00 local.
    const nairobi = { start: '22:00', end: '07:00', timezone: 'Africa/Nairobi' };
    expect(isWithinQuietHours(nairobi, at('2026-10-03T20:00:00Z'))).toBe(true);
    expect(isWithinQuietHours(nairobi, at('2026-10-03T17:00:00Z'))).toBe(false); // 20:00 local
    expect(isWithinQuietHours(nairobi, at('2026-10-04T03:59:00Z'))).toBe(true); // 06:59 local
    expect(isWithinQuietHours(nairobi, at('2026-10-04T04:00:00Z'))).toBe(false); // 07:00 local
  });

  it('treats equal start and end as no quiet hours', () => {
    expect(isWithinQuietHours({ start: '08:00', end: '08:00', timezone: 'UTC' }, at('2026-10-03T08:00:00Z'))).toBe(false);
  });

  it('ignores malformed input instead of silencing alerts', () => {
    expect(isWithinQuietHours({ start: '25:00', end: '07:00', timezone: 'UTC' }, at('2026-10-03T23:00:00Z'))).toBe(false);
    expect(isWithinQuietHours({ start: '22:00', end: '07:00', timezone: 'Not/AZone' }, at('2026-10-03T23:00:00Z'))).toBe(false);
  });
});

describe('time helpers', () => {
  it('validates HH:MM', () => {
    expect(isValidTime('00:00')).toBe(true);
    expect(isValidTime('23:59')).toBe(true);
    expect(isValidTime('24:00')).toBe(false);
    expect(isValidTime('7:00')).toBe(false);
  });

  it('reports midnight as 0 minutes', () => {
    expect(minutesInTimeZone(at('2026-10-03T00:00:00Z'), 'UTC')).toBe(0);
  });
});
