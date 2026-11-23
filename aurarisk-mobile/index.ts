// Side-effect imports first: background tasks and the notification handler must be defined in the
// global scope so they exist when the OS launches the app headless (no React tree mounted).
import './src/background/backgroundRefresh';
import './src/notifications/setup';

// Last, so the above is in place before Expo Router registers the app and renders src/app.
import 'expo-router/entry';
