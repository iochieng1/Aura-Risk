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

  it('measures the note limit in UTF-8 bytes like the server', () => {
    const note = 'é'.repeat(1001); // 2002 bytes, 1001 characters
    expect(utf8ByteLength(note)).toBe(2002);
    expect(validateNewReport({ ...valid, note }).ok).toBe(false);
  });
});
