import { useEffect } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { Stack } from 'expo-router';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';
import { Button } from '../components/Button';
import { ErrorNotice } from '../components/ErrorNotice';
import { useStore } from '../lib/store';
import { session } from '../services';
import { useTheme } from '../theme';
import { startLocale, t, useLocale } from '../i18n';

export default function RootLayout() {
  const theme = useTheme();
  const { user, ready, restoreError } = useStore(session.state);

  // Every screen follows the language: each one calls useLocale, so a change re-renders them
  // in place and keeps the navigation and what was typed.
  useLocale();

  // Trade the saved refresh token for a session once, when the app starts, and read the language.
  useEffect(() => {
    void startLocale();
    void session.restore();
  }, []);

  if (!ready || restoreError) {
    return (
      <SafeAreaProvider>
        <View style={{ flex: 1, justifyContent: 'center', padding: 16, backgroundColor: theme.bg }}>
          {restoreError ? (
            <View style={{ gap: 12 }}>
              <ErrorNotice message={restoreError} onRetry={() => void session.restore()} />
              <Button label={t('Sign out')} variant="secondary" onPress={() => void session.signOut()} />
            </View>
          ) : (
            <ActivityIndicator accessibilityLabel={t('Restoring your session')} />
          )}
        </View>
      </SafeAreaProvider>
    );
  }

  // The guard: the API is the real one, and this keeps the screens to signed-in users.
  return (
    <SafeAreaProvider>
      <StatusBar style="auto" />
      <Stack screenOptions={{ headerShown: false }}>
        <Stack.Protected guard={!user}>
          <Stack.Screen name="(auth)" />
        </Stack.Protected>
        <Stack.Protected guard={!!user}>
          <Stack.Screen name="(app)" />
        </Stack.Protected>
      </Stack>
    </SafeAreaProvider>
  );
}
