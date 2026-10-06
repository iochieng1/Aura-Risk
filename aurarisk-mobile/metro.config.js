// Learn more https://docs.expo.dev/guides/customizing-metro
const path = require('path');
const { getDefaultConfig } = require('expo/metro-config');

const config = getDefaultConfig(__dirname);

// @aurarisk/shared is linked from ../packages/shared (a file: dependency). Metro only sees files
// inside watchFolders, so add it explicitly. That folder has no node_modules of its own, so imports
// from it (including the Babel helpers injected while transforming it) resolve from this app's.
config.watchFolders = [...(config.watchFolders ?? []), path.resolve(__dirname, '../packages/shared')];
config.resolver.nodeModulesPaths = [path.resolve(__dirname, 'node_modules')];

module.exports = config;
