import { ScrollView, StyleSheet, Text } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Button } from '../../../components/Button';
import { WorkspaceSwitcher } from '../../../components/WorkspaceSwitcher';
import { useStore } from '../../../lib/store';
import { session, workspaces } from '../../../services';
import { useTheme } from '../../../theme';
import { t } from '../../../i18n';
import { LanguageSwitcher } from '../../../components/LanguageSwitcher';

export default function Home() {
  const theme = useTheme();
  const { user } = useStore(session.state);
  const { current } = useStore(workspaces.state);
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }}>
      <ScrollView contentContainerStyle={styles.scroll}>
        <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
          {current?.name ?? t('Workspace')}
        </Text>
        <Text style={{ color: theme.text2 }}>{t('Signed in as {name} ({email}). Contacts, companies, and the pipeline are in the tabs below.', { name: user?.name ?? '', email: user?.email ?? '' })}</Text>
        <Text accessibilityRole="header" style={[styles.heading, { color: theme.text }]}>
          {t('Workspaces')}
        </Text>
        <WorkspaceSwitcher />
        <LanguageSwitcher />
        <Button label={t('Sign out')} variant="secondary" onPress={() => void session.signOut()} />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: 16, gap: 16, maxWidth: 640, width: '100%', alignSelf: 'center' },
  title: { fontSize: 28, fontWeight: '700' },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 8 }
});
