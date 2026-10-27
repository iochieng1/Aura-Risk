import { KENYA_COUNTIES } from '../constants';
import { parseCoordinates, utf8ByteLength, validateNewReport } from '../validation';

describe('parseCoordinates', () => {
  it('accepts values in range', () => {
    expect(parseCoordinates(' -1.29 ', '36.82')).toEqual({ ok: true, value: { lat: -1.29, lon: 36.82 } });
  });

  it('treats blank input as an error, not zero', () => {
    const result = parseCoordinates('', '  ');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(Object.keys(result.errors).sort()).toEqual(['lat', 'lon']);
  });

  it('rejects out-of-range and non-numeric values', () => {
    expect(parseCoordinates('91', '0').ok).toBe(false);
    expect(parseCoordinates('0', '-180.5').ok).toBe(false);
    expect(parseCoordinates('abc', '0').ok).toBe(false);
  });
});

describe('validateNewReport', () => {
  const valid = {
    location: { name: '  Bridge ', lat: -1.29, lon: 36.82 },
    category: 'flooding',
    note: ' Water rising ',
  };

  it('trims text fields', () => {
    const result = validateNewReport(valid);
    expect(result).toEqual({
      ok: true,
      value: { location: { name: 'Bridge', lat: -1.29, lon: 36.82 }, category: 'flooding', note: 'Water rising' },
    });
  });

  it('reports every invalid field', () => {
    const result = validateNewReport({ location: { name: ' ', lat: 100, lon: 0 }, category: 'fire', note: '' });
    expect(result.ok).toBe(false);
    if (!result.ok) expect(Object.keys(result.errors).sort()).toEqual(['category', 'lat', 'name', 'note']);
  });

  it('accepts a county and ward, trimmed', () => {
    const result = validateNewReport({ ...valid, county: ' Kisumu ', ward: ' Nyalenda A ' });
    expect(result.ok && result.value.county).toBe('Kisumu');
    expect(result.ok && result.value.ward).toBe('Nyalenda A');
  });

  it('rejects an unknown county and a ward without a county', () => {
    const unknown = validateNewReport({ ...valid, county: 'Nyanza' });
    expect(!unknown.ok && unknown.errors.county).toBeTruthy();
    const orphan = validateNewReport({ ...valid, ward: 'Nyalenda A' });
    expect(!orphan.ok && orphan.errors.ward).toBeTruthy();
  });

  it('lists all 47 counties once', () => {
    expect(new Set(KENYA_COUNTIES).size).toBe(47);
  });

  it('measures the note limit in UTF-8 bytes like the server', () => {
    const note = 'é'.repeat(1001); // 2002 bytes, 1001 characters
    expect(utf8ByteLength(note)).toBe(2002);
    expect(validateNewReport({ ...valid, note }).ok).toBe(false);
  });
});
