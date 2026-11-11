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

// Network calls made from background tasks must finish well inside the OS budget (~30s on iOS).
export const REQUEST_TIMEOUT_MS = 10_000;

export const fetchWithTimeout = async (
  url: string,
  init: RequestInit = {},
  timeoutMs = REQUEST_TIMEOUT_MS
): Promise<Response> => {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
};
