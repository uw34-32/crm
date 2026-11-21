import { ScrollView, StyleSheet, Text } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Button } from '../../../components/Button';
import { WorkspaceSwitcher } from '../../../components/WorkspaceSwitcher';
import { useStore } from '../../../lib/store';
import { session, workspaces } from '../../../services';
import { useTheme } from '../../../theme';

export default function Home() {
  const theme = useTheme();
  const { user } = useStore(session.state);
  const { current } = useStore(workspaces.state);
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }}>
      <ScrollView contentContainerStyle={styles.scroll}>
        <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
          {current?.name ?? 'Workspace'}
        </Text>
        <Text style={{ color: theme.text2 }}>Signed in as {user?.name} ({user?.email}). Contacts, companies, and the pipeline are in the tabs below.</Text>
        <Text accessibilityRole="header" style={[styles.heading, { color: theme.text }]}>
          Workspaces
        </Text>
        <WorkspaceSwitcher />
        <Button label="Sign out" variant="secondary" onPress={() => void session.signOut()} />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: 16, gap: 16, maxWidth: 640, width: '100%', alignSelf: 'center' },
  title: { fontSize: 28, fontWeight: '700' },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 8 }
});
