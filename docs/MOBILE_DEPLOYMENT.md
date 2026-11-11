# Running AuraRisk on an Android phone

How to get the AuraRisk mobile app (`aurarisk-mobile/`, Expo SDK 57) onto a
physical Android phone, talking to the live demo API. The server side (Render
+ Neon) is already deployed and is covered in [DEPLOYMENT.md](DEPLOYMENT.md);
nothing here changes it.

## The short version

| Route | Time | What you get | Use it for |
|---|---|---|---|
| **A. EAS preview APK** (recommended) | ~1 h the first time, mostly waiting for the build | A real installed app with its own icon; works without your laptop | Showing AuraRisk on your phone |
| **B. `expo start` against the live API** | ~10 min | The app running from your laptop in Expo Go or a development build | Trying changes quickly |
| C. Web app in Chrome | 1 min | `aurarisk-web.onrender.com` → ⋮ → *Add to Home screen* | A fallback. It's a shortcut, not an installable app, until [#83](https://github.com/iochieng1/Aura-Risk/issues/83) |

| Live service | URL |
|---|---|
| API (the app talks to this) | `https://aurarisk-api.onrender.com` |
| Web app | `https://aurarisk-web.onrender.com` |

Point the app at the **API** URL, not the web one.

## Before you start: known blockers

| Blocker | Effect | Tracked in | Workaround for now |
|---|---|---|---|
| No `eas.json`, no `android.package`, no EAS project ID | Can't build an APK, and push can't get a token | [#79](https://github.com/iochieng1/Aura-Risk/issues/79) | Steps 2–4 below create them |
| `src/config.ts` falls back to `http://localhost:8080` when `EXPO_PUBLIC_API_BASE_URL` is unset | A build without the variable silently points at the phone itself, and every request fails | [#79](https://github.com/iochieng1/Aura-Risk/issues/79) | Set the variable in the build profile (step 4) |
| Mobile requests time out after 10 s, but the free API takes up to ~60 s to wake | The first launch after a quiet period shows errors | [#93](https://github.com/iochieng1/Aura-Risk/issues/93) | Wake the API first (smoke test, step 1) |
| Photos are off on the demo (no S3/ClamAV); push alerts are off (`NOTIFIER_INTERVAL=0`, no Expo token) | Photo attach fails with 503; no alerts arrive | [#88](https://github.com/iochieng1/Aura-Risk/issues/88), [#82](https://github.com/iochieng1/Aura-Risk/issues/82) | Leave these out of the demo |
| Queued reports are dropped silently after 3 failed sends | A report with a failing photo can vanish from the queue | [#80](https://github.com/iochieng1/Aura-Risk/issues/80) | Don't attach photos on the demo |

Things that are **not** blockers:
- **CORS:** it only applies to browsers, so the native app is unaffected.
- **Mock data:** the mobile app has no mock mode, and the web app's mock mode
  only exists in dev builds.
- **API keys:** none are needed on the phone. The app uses Open-Meteo,
  Nominatim and the AuraRisk API without keys.
- **HTTPS:** Android release builds block plain `http://`. The Render URL is
  HTTPS, so this is fine. It only matters if you point a build at a laptop.

## Prerequisites

- Node 24 and npm (same as CI).
- A free Expo account: <https://expo.dev/signup>.
- EAS CLI: `npm install -g eas-cli`, or prefix commands with `npx eas-cli@latest`.
- An Android phone with internet access, and permission to install apps from
  your browser (Settings → Apps → Special app access → *Install unknown apps*).

## Route A: build and install an APK with EAS (recommended)

### 1. Smoke-test and wake the API

```bash
curl -s https://aurarisk-api.onrender.com/health/ready
# {"checks":{"database":"ok","weather":"ok"},"status":"ok"}

# Kisumu
curl -s "https://aurarisk-api.onrender.com/api/risk?lat=-0.0917&lon=34.7680" | head -c 400
```

If the first call hangs, wait up to a minute (the server is waking) and try
again. A `503` from `/api/risk` with a "weather unavailable" message is
expected sometimes on the shared free IP: the app then fetches the weather on
the phone and posts it to `POST /api/risk`
(see [DEPLOYMENT.md](DEPLOYMENT.md#3-check-it)).

### 2. Install dependencies and check the project

```bash
cd aurarisk-mobile
npm ci
npx expo-doctor
npx tsc --noEmit && npx jest --ci
```

### 3. Give the app an identity (`app.json`)

Edit `aurarisk-mobile/app.json`. Use your own reverse-domain ID, and don't
change it after the first install, because Android treats a new ID as a
different app.

```jsonc
{
  "expo": {
    "name": "AuraRisk",                 // was "aurarisk-mobile": the name under the icon
    "slug": "aurarisk-mobile",          // keep: the EAS project is keyed on it
    "version": "1.0.0",
    "android": {
      "package": "com.<your-handle>.aurarisk",
      // keep the existing adaptiveIcon and predictiveBackGestureEnabled
    },
    "ios": {
      "bundleIdentifier": "com.<your-handle>.aurarisk",
      // keep the existing supportsTablet and config
    }
    // keep "plugins" as they are
  }
}
```

Then link the project to EAS:

```bash
eas login
eas init
```

`eas init` creates the project on expo.dev and writes
`expo.extra.eas.projectId` (and `owner`) into `app.json`.
`src/notifications/push.ts` reads that ID to get a push token. Without it,
the app reports push as unavailable.

### 4. Create `eas.json` with the API URL per profile

Run `eas build:configure`, choose **Android**, and then make
`aurarisk-mobile/eas.json` look like this:

```json
{
  "cli": { "appVersionSource": "remote" },
  "build": {
    "development": {
      "developmentClient": true,
      "distribution": "internal",
      "android": { "buildType": "apk" },
      "env": { "EXPO_PUBLIC_API_BASE_URL": "https://aurarisk-api.onrender.com" }
    },
    "preview": {
      "distribution": "internal",
      "android": { "buildType": "apk" },
      "env": { "EXPO_PUBLIC_API_BASE_URL": "https://aurarisk-api.onrender.com" }
    },
    "production": {
      "autoIncrement": true,
      "env": { "EXPO_PUBLIC_API_BASE_URL": "https://<your-production-api-host>" }
    }
  }
}
```

- `EXPO_PUBLIC_*` values are **inlined into the JavaScript bundle at build
  time**. Changing the URL means a new build (or an EAS Update), not a
  setting on the phone.
- They are public by design, so never put a secret in an `EXPO_PUBLIC_`
  variable. Nothing in the app needs one.
- `preview` builds an `.apk` you can install directly; `production` builds
  an `.aab` for Google Play ([#90](https://github.com/iochieng1/Aura-Risk/issues/90)).
- Instead of `env` in `eas.json`, you can use EAS environment variables:
  `eas env:create --name EXPO_PUBLIC_API_BASE_URL --value https://aurarisk-api.onrender.com --environment preview --visibility plaintext`.

**Monorepo note:** the app depends on `file:../packages/shared`. EAS uploads
the whole git repository (respecting `.gitignore`) and builds inside
`aurarisk-mobile/`, and `metro.config.js` already watches
`../packages/shared`, so no extra setup is needed. Run the build from
`aurarisk-mobile/`.

### 5. Build

```bash
cd aurarisk-mobile
eas build -p android --profile preview
```

- The first build asks to generate an Android keystore. Answer yes; EAS
  stores it for you.
- On the free plan the build waits in a queue, usually 10–30 minutes. Watch
  progress on expo.dev or in the terminal.
- When it finishes, the CLI prints a build URL and a QR code.

**Local alternative:** if you have Android Studio and the Android SDK, run
`eas build -p android --profile preview --local`. It runs the same build on
your machine and skips the queue.

### 6. Install on the phone

1. Scan the QR code with the phone's camera, or open the build URL in Chrome
   on the phone, and tap **Install**/**Download**.
2. Open the downloaded `.apk`. If Android asks, allow Chrome to install
   unknown apps.
3. If Play Protect warns that the app is unrecognised, choose **More
   details → Install anyway**. That's expected for apps not from the Play
   Store.
4. Open **AuraRisk** from the app drawer.

With a USB cable and `adb`, `adb install path/to/build.apk` works too.
`eas build:run -p android --latest` installs the latest build on a connected
device or emulator.

Updating: build again and install over the old app. The `android.package`
and the EAS keystore must stay the same, or Android refuses the update.

## Route B: `expo start` against the live API

Best for trying changes quickly. Your laptop serves the JavaScript; the phone
talks to the live API directly.

```bash
cd aurarisk-mobile
npm ci
EXPO_PUBLIC_API_BASE_URL=https://aurarisk-api.onrender.com npx expo start --tunnel
```

Or put `EXPO_PUBLIC_API_BASE_URL=https://aurarisk-api.onrender.com` in
`aurarisk-mobile/.env`, which is git-ignored, and run `npx expo start`.
`--tunnel` works when the phone and laptop are on different networks; leave
it off on the same Wi-Fi.

Which client to open it in:
- **Expo Go** (Play Store): scan the QR code. Expo Go only runs the SDK
  version it was built for. If it says the project is incompatible, it has
  moved past SDK 57; use a development build instead. Push notifications
  don't work in Expo Go, and background tasks are limited.
- **Development build:** run `eas build -p android --profile development`
  once, install the APK as in step 6, then `npx expo start --dev-client` and
  open the project from the dev client. It has every native module the app
  uses, including push.

To point the app at a backend on your laptop instead, use your laptop's LAN
address (for example `http://192.168.1.20:8080`), not `localhost`.
Development builds and Expo Go allow plain HTTP; release APKs don't.

## Smoke test on the phone

Do these in order on a fresh install, after waking the API (step 1):

1. The app opens without a red error screen. There is no "waiting to send"
   banner yet.
2. **Use my location**: allow location access, and a risk panel appears with
   a score, a level, tips and a data-freshness line.
3. **Search** "Kisumu", select a result, and the risk updates for that place.
4. The nearby reports list loads (it may be empty).
5. Submit a test report (no photo), and it appears in the list after
   refreshing.

If step 2 or 3 shows an error, see Troubleshooting.

## Demo checklist

Features that work against the demo API:

- [ ] **Location by GPS**, or by search when location permission is denied.
- [ ] **Flood risk for Kisumu** (try Ahero or Kano plains too): score
      0–100, Advisory/Alert/Emergency level, tips, and the confidence
      reasons, including when weather came from the device fallback.
- [ ] **"Decision support only"** wording and the Open-Meteo credit.
- [ ] **Swahili**: tap the language toggle, and the UI and tips switch.
- [ ] **Community report** with county and ward, then show it in the nearby
      list.
- [ ] **Offline**: turn on aeroplane mode, then
  - the cached risk still shows, marked stale;
  - submitting a report queues it, with a "waiting to send" banner;
  - turning aeroplane mode off sends it automatically.
- [ ] **Push consent flow**: the in-app explanation, then the OS permission
      prompt. Alerts won't arrive on the demo (notifier off, #82).
- [ ] **Privacy**: location is only requested when you tap *Use my location*; search and typed coordinates work without it. (Account deletion exists in the API and auth client, but there is no button for it in the app yet.)
- [ ] Optional, in the phone's browser: the **county dashboard** on
      `aurarisk-web.onrender.com`.

Leave out on the demo:
- **Photo attachments:** the API answers "photo uploads are not configured".
- **Expecting a real push alert.**

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Every request fails right after install | Built without `EXPO_PUBLIC_API_BASE_URL`, so it uses `http://localhost:8080` | Check the `env` block of the profile you built; rebuild |
| Errors on the first launch, fine a minute later | Free API was asleep; the app gives up after 10 s (#93) | Open `https://aurarisk-api.onrender.com/health/ready` first, then select the location again |
| "Weather unavailable" or confidence notes about device weather | Open-Meteo's free daily limit on Render's shared IP | Expected: the app falls back to fetching weather on the phone |
| Requests to a `http://` URL fail only in the APK | Android blocks cleartext HTTP in release builds | Use the HTTPS API URL |
| "App not installed" when updating | Different `android.package` or signing key than the installed copy | Uninstall the old app, or keep the package and EAS keystore unchanged |
| Play Protect blocks the install | Unrecognised side-loaded app | *More details → Install anyway* |
| Expo Go: "project is incompatible with this version of Expo Go" | Expo Go has moved past SDK 57 | Use a development build (Route B) |
| Push shows "EAS projectId is not configured" | `eas init` not run, or the build predates it | Run `eas init`, rebuild |
| Push enabled but no alerts arrive | Demo notifier is off (`NOTIFIER_INTERVAL=0`) | Expected on the demo; see #82 |
| Place search returns nothing | Fewer than 3 characters, or Nominatim's 1 request/second limit | Type a longer name and press search once |
| Photo upload fails with 503 | Demo has no S3/ClamAV | Expected; skip photos on the demo |
| `eas build` fails resolving `@aurarisk/shared` | Build started outside the repo, or `packages/shared` is git-ignored or uncommitted | Run from `aurarisk-mobile/` inside the git checkout |
| Build stuck in the queue | Free-plan queue | Wait, or use `--local` |

## After the demo

Making this repeatable is tracked in #79, #93, #80, #81 and #82. To be on
Google Play, see #90. The full plan is in
[PRODUCTION_ROADMAP.md](PRODUCTION_ROADMAP.md).
