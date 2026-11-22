import { useEffect } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { Redirect, Stack, usePathname } from 'expo-router';
import { ErrorNotice } from '../../components/ErrorNotice';
import { useStore } from '../../lib/store';
import { session, workspaces } from '../../services';
import { useTheme } from '../../theme';
import { t } from '../../i18n';

export default function AppLayout() {
  const theme = useTheme();
  const { user } = useStore(session.state);
  const { status, error, list, userId } = useStore(workspaces.state);
  const pathname = usePathname();

  // Load the workspaces once per signed-in user. Sign-out forgets them in bind.ts.
  useEffect(() => {
    if (user && userId !== user.id) void workspaces.load(user.id);
  }, [user, userId]);

  if (status === 'error') {
    return (
      <View style={{ flex: 1, justifyContent: 'center', padding: 16, backgroundColor: theme.bg }}>
        <ErrorNotice message={error} onRetry={() => user && void workspaces.load(user.id)} />
      </View>
    );
  }
  if (status !== 'ready') {
    return (
      <View style={{ flex: 1, justifyContent: 'center', backgroundColor: theme.bg }}>
        <ActivityIndicator accessibilityLabel={t('Loading your workspaces')} />
      </View>
    );
  }
  // A user with no workspace has nothing to open until they create one.
  return (
    <>
      {list.length === 0 && pathname !== '/workspaces/new' ? <Redirect href="/workspaces/new" /> : null}
      <Stack screenOptions={{ headerShown: false }}>
        <Stack.Screen name="(tabs)" />
        <Stack.Screen name="workspaces/new" />
        <Stack.Screen name="contacts/new" />
        <Stack.Screen name="contacts/[id]/index" />
        <Stack.Screen name="contacts/[id]/edit" />
        <Stack.Screen name="companies/new" />
        <Stack.Screen name="companies/[id]/index" />
        <Stack.Screen name="companies/[id]/edit" />
        <Stack.Screen name="deals/new" />
        <Stack.Screen name="deals/[id]/index" />
        <Stack.Screen name="deals/[id]/edit" />
      </Stack>
    </>
  );
}
