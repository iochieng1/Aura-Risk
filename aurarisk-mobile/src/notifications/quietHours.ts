import { QuietHours } from '../types/api';

const TIME_PATTERN = /^([01]\d|2[0-3]):([0-5]\d)$/;

export const isValidTime = (value: string): boolean => TIME_PATTERN.test(value);

export const isValidTimeZone = (timeZone: string): boolean => {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone });
    return true;
  } catch {
    return false;
  }
};

export const deviceTimeZone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
};

const toMinutes = (value: string): number => {
  const match = TIME_PATTERN.exec(value);
  if (!match) throw new RangeError(`Invalid time "${value}"`);
  return Number(match[1]) * 60 + Number(match[2]);
};

/** Minutes since local midnight for `now` in the given IANA time zone. */
export const minutesInTimeZone = (now: Date, timeZone: string): number => {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(now);
  const hour = Number(parts.find((p) => p.type === 'hour')?.value ?? 0) % 24;
  const minute = Number(parts.find((p) => p.type === 'minute')?.value ?? 0);
  return hour * 60 + minute;
};

/**
 * True when `now` falls inside the quiet window, evaluated in the window's own time zone.
 * The window includes `start` and excludes `end`. A window with `end` earlier than `start` spans
 * midnight (e.g. 22:00-07:00). `start === end` is treated as an empty window. Same semantics as
 * the server-side notifier.
 */
export const isWithinQuietHours = (quietHours: QuietHours | null | undefined, now: Date = new Date()): boolean => {
  if (!quietHours) return false;
  if (!isValidTime(quietHours.start) || !isValidTime(quietHours.end) || !isValidTimeZone(quietHours.timezone)) {
    return false;
  }

  const start = toMinutes(quietHours.start);
  const end = toMinutes(quietHours.end);
  if (start === end) return false;

  const current = minutesInTimeZone(now, quietHours.timezone);
  return start < end ? current >= start && current < end : current >= start || current < end;
};
