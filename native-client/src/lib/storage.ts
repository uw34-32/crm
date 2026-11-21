// The device's key-value storage: the secure store on iOS and Android, localStorage on
// the web export. Every caller copes with storage that throws.

import { Platform } from 'react-native';
import * as SecureStore from 'expo-secure-store';
import type { AsyncStorage } from '../api/tokens';

const secure: AsyncStorage = {
  get: (key) => SecureStore.getItemAsync(key),
  set: (key, value) => SecureStore.setItemAsync(key, value),
  remove: (key) => SecureStore.deleteItemAsync(key)
};

const web: AsyncStorage = {
  get: async (key) => window.localStorage.getItem(key),
  set: async (key, value) => window.localStorage.setItem(key, value),
  remove: async (key) => window.localStorage.removeItem(key)
};

export const deviceStorage: AsyncStorage = Platform.OS === 'web' ? web : secure;
