// The web app's palette, so the clients read as one product.

import { useColorScheme } from 'react-native';

const light = {
  bg: '#f2f2f7',
  surface: '#ffffff',
  surface2: '#e5e5ea',
  text: '#1c1c1e',
  text2: '#5f5f63',
  border: '#d1d1d6',
  accent: '#0a84ff',
  accentText: '#005bb8',
  accentStrong: '#0066cc',
  danger: '#ff453a',
  dangerText: '#b3261e',
  dangerStrong: '#b3261e'
};

const dark: typeof light = {
  ...light,
  bg: '#000000',
  surface: '#1c1c1e',
  surface2: '#2c2c2e',
  text: '#ffffff',
  text2: '#9a9a9f',
  border: '#38383a',
  accentText: '#4da3ff',
  dangerText: '#ff7a72'
};

export type Theme = typeof light;
export const radius = 12;

export function useTheme(): Theme {
  return useColorScheme() === 'dark' ? dark : light;
}
