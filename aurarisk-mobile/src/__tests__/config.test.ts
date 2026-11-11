import { resolveApiBaseUrl } from '../config';

describe('resolveApiBaseUrl', () => {
  it('uses the configured URL and strips trailing slashes', () => {
    expect(resolveApiBaseUrl('https://api.example.com/', false)).toBe('https://api.example.com');
    expect(resolveApiBaseUrl('  https://api.example.com  ', true)).toBe('https://api.example.com');
  });

  it('falls back to localhost only in development', () => {
    expect(resolveApiBaseUrl(undefined, true)).toBe('http://localhost:8080');
    expect(resolveApiBaseUrl('', true)).toBe('http://localhost:8080');
  });

  it('refuses to fall back to localhost in a release build', () => {
    expect(() => resolveApiBaseUrl(undefined, false)).toThrow(/EXPO_PUBLIC_API_BASE_URL/);
    expect(() => resolveApiBaseUrl('   ', false)).toThrow(/EXPO_PUBLIC_API_BASE_URL/);
  });
});
