# @aurarisk/shared

TypeScript shared by `aurarisk-frontend` (Vite) and `aurarisk-mobile` (Expo). It has no runtime
dependencies and ships as source, so there is no build step.

| Module | Contents |
|---|---|
| `types.ts` | Wire types for the backend JSON (`RiskAssessment`, `CommunityReport`, `NewReportInput`, subscriptions, photos, tokens) |
| `constants.ts` | Risk levels and colours, `scoreToLevel`, report categories, server-side length limits |
| `validation.ts` | `validateNewReport`, `validateLocation`, `parseCoordinates`, and response guards (`isRiskAssessment`, `isCommunityReport`) |
| `client.ts` | `createApiClient({ baseUrl, fetch })` for `GET /api/risk`, `GET /api/reports`, `POST /api/reports`, plus `ApiError` / `ValidationError` |
| `geocoding.ts` | `searchPlaces` (OpenStreetMap Nominatim) |

The validators mirror the Go handlers in `aurarisk-backend/internal/handlers`, and the server
remains the source of truth. If you change a JSON field or a limit in the backend, change it here too.

## Consuming it

Both apps depend on it as `"@aurarisk/shared": "file:../packages/shared"`, which npm installs as a
symlink. Because the package lives outside each app's root:

- **Vite** allows the folder via `server.fs.allow` in `vite.config.ts`.
- **Metro** adds it to `watchFolders` and resolves its imports from the app's `node_modules`
  (`metro.config.js`).
- **Jest** (mobile) runs this package's tests through `roots` and `modulePaths` in `package.json`.

## Tests

```bash
cd aurarisk-mobile && npx jest ../packages/shared
```
