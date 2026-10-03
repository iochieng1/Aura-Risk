import { registerRootComponent } from 'expo';

// Side-effect imports first: background tasks and the notification handler must be defined in the
// global scope so they exist when the OS launches the app headless (no React tree mounted).
import './src/background/backgroundRefresh';
import './src/notifications/setup';

import App from './App';

// registerRootComponent calls AppRegistry.registerComponent('main', () => App);
// It also ensures that whether you load the app in Expo Go or in a native build,
// the environment is set up appropriately
registerRootComponent(App);
