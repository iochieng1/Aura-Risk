/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Backend origin, e.g. "https://api.example.org". Empty means same origin. */
  readonly VITE_API_BASE_URL?: string;
  /** "true" serves mock data instead of calling the backend. Development only. */
  readonly VITE_USE_MOCK_API?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
