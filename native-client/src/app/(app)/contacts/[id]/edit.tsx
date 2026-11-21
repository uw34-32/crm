import { useState } from 'react';
import { ActivityIndicator, StyleSheet, Text } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { recordsApi } from '../../../../api/records';
import { ContactForm } from '../../../../components/ContactForm';
import { ErrorNotice } from '../../../../components/ErrorNotice';
import { NotFound } from '../../../../components/NotFound';
import { ScreenFrame } from '../../../../components/ScreenFrame';
import { recordsChanged } from '../../../../lib/records/changes';
import { contactPatchBody, type ContactDraft } from '../../../../lib/records/fields';
import { useLeaveOnWorkspaceSwitch, useRecord } from '../../../../lib/records/hooks';
import { isMissing } from '../../../../lib/records/load';
import { session } from '../../../../services';
import { useTheme } from '../../../../theme';

const contacts = recordsApi(session.api, 'contacts');

export default function EditContact() {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  useLeaveOnWorkspaceSwitch('/contacts');
  const { state, workspaceId, reload } = useRecord(contacts.get, id);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  if (state.status === 'missing') return <NotFound title="Contact not found" backHref="/contacts" backLabel="contacts" />;
  const contact = state.record;

  async function save(draft: ContactDraft) {
    if (!contact || !workspaceId) return;
    const body = contactPatchBody(contact, draft);
    // Nothing changed: the API refuses an empty PATCH, and there is nothing to save.
    if (Object.keys(body).length === 0) return router.back();
    setBusy(true);
    setError('');
    try {
      await contacts.update(workspaceId, contact.id, body);
      recordsChanged();
      router.back();
    } catch (err) {
      setError(
        isMissing(err) ? 'This contact, or the chosen company, no longer exists. Go back and check the list.' : err instanceof Error ? err.message : String(err)
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScreenFrame backLabel="Contact" backHref={`/contacts/${id}`}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        Edit contact
      </Text>
      {state.status === 'error' ? (
        <ErrorNotice message={state.error} onRetry={() => void reload()} />
      ) : !contact || !workspaceId ? (
        <ActivityIndicator accessibilityLabel="Loading the contact" />
      ) : (
        <ContactForm
          workspaceId={workspaceId}
          initial={{
            first_name: contact.first_name,
            last_name: contact.last_name ?? '',
            email: contact.email ?? '',
            phone: contact.phone ?? '',
            title: contact.title ?? '',
            company: contact.company_id ? { id: contact.company_id, name: contact.company_name ?? '' } : null
          }}
          submitLabel="Save changes"
          busy={busy}
          error={error}
          onSubmit={(d) => void save(d)}
          onCancel={() => router.back()}
        />
      )}
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({ title: { fontSize: 28, fontWeight: '700' } });
