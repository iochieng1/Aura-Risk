# decode-uri-component (AuraRisk stand-in)

expo-router 57 parses deep links with `query-string@7`, which depends on
`decode-uri-component@^0.2.2`. Every release up to 0.5.0 has denial-of-service
advisories (exponential and quadratic decoding of malformed percent-encoded
input). The only fixed release, 0.5.1, is ESM-only, and `query-string@7` loads it
with `require()` and calls it as a function, so it cannot be swapped in.

This package keeps the old CommonJS contract (decode as much as possible, never
throw on malformed input) with a linear-time algorithm. `package.json` points
`decode-uri-component` here through `overrides`. Tests:
`src/__tests__/decodeUriComponent.test.ts`.

Remove this folder and the override once expo-router no longer depends on a
vulnerable `decode-uri-component` (expo-router 58 / Expo SDK 58).
