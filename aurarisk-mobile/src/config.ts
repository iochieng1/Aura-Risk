// Release builds get EXPO_PUBLIC_API_BASE_URL from eas.json, and app.config.ts
// fails the build if it is missing. The localhost fallback is for `expo start`
// during development only, so a release build never points at the device itself.
const DEV_FALLBACK_API_BASE_URL = 'http://localhost:8080';

export const resolveApiBaseUrl = (configured: string | undefined, isDev: boolean): string => {
  const url = configured?.trim().replace(/\/+$/, '');
  if (url) return url;
  if (isDev) return DEV_FALLBACK_API_BASE_URL;
  throw new Error('EXPO_PUBLIC_API_BASE_URL is not set for this release build.');
};

export const API_BASE_URL = resolveApiBaseUrl(process.env.EXPO_PUBLIC_API_BASE_URL, __DEV__);

/** Fire-and-forget request that starts waking a sleeping API as soon as the app opens. */
export const warmUpServer = (send: (url: string) => Promise<unknown>): void => {
  send(`${API_BASE_URL}/health`).catch(() => {
    // Best effort: real requests retry on their own.
  });
};
