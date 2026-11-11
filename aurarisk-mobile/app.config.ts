import type { ConfigContext, ExpoConfig } from 'expo/config';

// EXPO_PUBLIC_* values are inlined into the JS bundle at build time, so a
// release build without a reachable HTTPS API is unusable. Fail the EAS build
// early instead of shipping an app that silently talks to localhost.
const RELEASE_PROFILES = new Set(['preview', 'production']);

const validateApiBaseUrl = (): void => {
  const profile = process.env.EAS_BUILD_PROFILE;
  if (!profile || !RELEASE_PROFILES.has(profile)) return;

  const raw = process.env.EXPO_PUBLIC_API_BASE_URL;
  let url: URL;
  try {
    url = new URL(raw ?? '');
  } catch {
    throw new Error(
      `EXPO_PUBLIC_API_BASE_URL must be set to the public API URL for the "${profile}" build profile (see eas.json).`
    );
  }
  const localHosts = ['localhost', '127.0.0.1', '10.0.2.2', '0.0.0.0'];
  if (url.protocol !== 'https:' || localHosts.includes(url.hostname)) {
    throw new Error(
      `EXPO_PUBLIC_API_BASE_URL must be a public https:// URL for the "${profile}" build profile, got "${raw}".`
    );
  }
};

export default ({ config }: ConfigContext): ExpoConfig => {
  validateApiBaseUrl();
  return config as ExpoConfig;
};
