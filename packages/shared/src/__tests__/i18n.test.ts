import { describeFreshness } from '../freshness';
import { categoryLabel, MESSAGES, pickLanguage, reportStatusLabel, riskLevelLabel, translate } from '..';

const placeholders = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort();

describe('translation catalogue', () => {
  const keys = Object.keys(MESSAGES.en) as (keyof typeof MESSAGES.en)[];

  it.each(Object.keys(MESSAGES))('has every key, non-empty, in %s', (lang) => {
    const messages = MESSAGES[lang as keyof typeof MESSAGES];
    expect(Object.keys(messages).sort()).toEqual([...keys].sort());
    for (const key of keys) expect(messages[key].trim()).not.toBe('');
  });

  it.each(Object.keys(MESSAGES))('keeps the English placeholders in %s', (lang) => {
    const messages = MESSAGES[lang as keyof typeof MESSAGES];
    for (const key of keys) expect([key, placeholders(messages[key])]).toEqual([key, placeholders(MESSAGES.en[key])]);
  });
});

describe('translate', () => {
  it('fills placeholders and leaves unknown ones', () => {
    expect(translate('sw', 'mobile.score', { score: 62 })).toBe('Alama 62/100');
    expect(translate('en', 'dash.range', { from: '2026-01-01' })).toBe('2026-01-01 to {to}');
  });

  it('labels levels, categories and report status', () => {
    expect(riskLevelLabel('sw', 'Emergency')).toBe('Dharura');
    expect(categoryLabel('sw', 'flooding')).toBe('Mafuriko');
    expect(categoryLabel('sw', 'unknown')).toBe('unknown');
    expect(reportStatusLabel({ status: 'verified', response_status: 'resolved' }, 'sw')).toBe(
      'Imethibitishwa · Kaunti imeshughulikia'
    );
    expect(reportStatusLabel({ status: 'verified' })).toBe('Verified · Awaiting county response');
  });

  it('describes freshness in Swahili', () => {
    const now = Date.parse('2026-10-28T12:00:00Z');
    const risk = {
      location: { name: 'x', lat: 0, lon: 0 },
      score: 1,
      level: 'Normal' as const,
      summary: '',
      tips: [],
      source_timestamps: { weather_fetched_at: '2026-10-28T11:55:00Z' },
    };
    expect(describeFreshness(risk, now, 'sw')?.label).toBe('Data ya hali ya hewa ya dakika 5 zilizopita');
  });
});

describe('pickLanguage', () => {
  it('takes the first supported primary subtag', () => {
    expect(pickLanguage(['sw-KE', 'en-GB'])).toBe('sw');
    expect(pickLanguage(['fr-FR', 'SW'])).toBe('sw');
    expect(pickLanguage(['en-KE', 'sw'])).toBe('en');
    expect(pickLanguage(['fr'])).toBe('en');
    expect(pickLanguage([])).toBe('en');
  });
});
