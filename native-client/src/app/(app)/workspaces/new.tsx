import { useState } from 'react';
import { ScrollView, StyleSheet, Text } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Button } from '../../../components/Button';
import { ErrorNotice } from '../../../components/ErrorNotice';
import { Field } from '../../../components/Field';
import { checkName, toSentence } from '../../../lib/forms';
import { useStore } from '../../../lib/store';
import { workspaces } from '../../../services';
import { useTheme } from '../../../theme';

export default function NewWorkspace() {
  const theme = useTheme();
  const { list } = useStore(workspaces.state);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [formError, setFormError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit() {
    setFormError('');
    const problem = checkName(name, 'Enter a name for the workspace.');
    setError(problem);
    if (problem) return;
    setBusy(true);
    try {
      await workspaces.create(name.trim());
      router.replace('/');
    } catch (err) {
      setFormError(toSentence(err instanceof Error ? err.message : String(err)));
    } finally {
      setBusy(false);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }}>
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
          New workspace
        </Text>
        {list.length === 0 ? <Text style={{ color: theme.text2 }}>You are not in a workspace yet. Create one to start.</Text> : null}
        {formError ? <ErrorNotice message={formError} /> : null}
        <Field label="Name" value={name} onChangeText={setName} error={error} returnKeyType="go" onSubmitEditing={() => void submit()} />
        <Button label={busy ? 'Creating…' : 'Create workspace'} busy={busy} onPress={() => void submit()} />
        {list.length > 0 ? <Button label="Cancel" variant="secondary" onPress={() => router.back()} /> : null}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: 16, gap: 16, maxWidth: 640, width: '100%', alignSelf: 'center' },
  title: { fontSize: 28, fontWeight: '700' }
});
