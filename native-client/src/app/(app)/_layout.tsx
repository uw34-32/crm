import { useEffect } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { Redirect, Stack } from 'expo-router';
import { ErrorNotice } from '../../components/ErrorNotice';
import { useStore } from '../../lib/store';
import { session, workspaces } from '../../services';
import { useTheme } from '../../theme';

export default function AppLayout() {
  const theme = useTheme();
  const { user } = useStore(session.state);
  const { status, error, list, userId } = useStore(workspaces.state);

  // Load the workspaces once per signed-in user; sign-out forgets them.
  useEffect(() => {
    if (!user) {
      workspaces.reset();
      return;
    }
    if (userId !== user.id) void workspaces.load(user.id);
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
        <ActivityIndicator accessibilityLabel="Loading your workspaces" />
      </View>
    );
  }
  // A user with no workspace has nothing to open until they create one.
  return (
    <>
      {list.length === 0 ? <Redirect href="/workspaces/new" /> : null}
      <Stack screenOptions={{ headerShown: false }} />
    </>
  );
}
