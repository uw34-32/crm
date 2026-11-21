import { useEffect } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { Stack } from 'expo-router';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';
import { ErrorNotice } from '../components/ErrorNotice';
import { useStore } from '../lib/store';
import { session } from '../services';
import { useTheme } from '../theme';

export default function RootLayout() {
  const theme = useTheme();
  const { user, ready, restoreError } = useStore(session.state);

  // Trade the saved refresh token for a session once, when the app starts.
  useEffect(() => {
    void session.restore();
  }, []);

  if (!ready || restoreError) {
    return (
      <SafeAreaProvider>
        <View style={{ flex: 1, justifyContent: 'center', padding: 16, backgroundColor: theme.bg }}>
          {restoreError ? (
            <ErrorNotice message={restoreError} onRetry={() => void session.restore()} />
          ) : (
            <ActivityIndicator accessibilityLabel="Restoring your session" />
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
