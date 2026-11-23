// The stand-in in vendor/decode-uri-component that query-string (used by expo-router to parse deep
// links) loads through the package.json override.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const decode: (input: string) => string = require('decode-uri-component');

describe('decode-uri-component stand-in', () => {
  it('is the vendored copy, not the vulnerable npm release', () => {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    expect(require('decode-uri-component/package.json').version).toBe('0.0.0-aurarisk.1');
  });

  it('decodes valid input like decodeURIComponent', () => {
    expect(decode('caf%C3%A9%20Nairobi')).toBe('café Nairobi');
    expect(decode('%F0%9F%8C%8A')).toBe('🌊');
    expect(decode('plain')).toBe('plain');
  });

  it('decodes what it can of malformed input and keeps the rest as written', () => {
    expect(decode('%')).toBe('%');
    expect(decode('%zz%C3%A9')).toBe('%zzé');
    expect(decode('%C3%A9%FF%41')).toBe('é%FFA');
    expect(decode('%C3')).toBe('%C3');
    expect(decode('a%E0%A4%A')).toBe('a%E0%A4%A');
  });

  it('throws a TypeError for non-strings, like the original', () => {
    expect(() => decode(42 as unknown as string)).toThrow(TypeError);
  });

  it('stays fast on long malformed runs (the advisories are exponential/quadratic blow-ups)', () => {
    const hostile = '%C3'.repeat(200_000) + '%';
    const started = Date.now();
    expect(decode(hostile)).toBe(hostile);
    expect(Date.now() - started).toBeLessThan(2_000);
  });
});
