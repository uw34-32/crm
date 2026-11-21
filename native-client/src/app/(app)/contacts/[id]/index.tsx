import { useState } from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { recordsApi } from '../../../../api/records';
import { ActivityTimeline } from '../../../../components/ActivityTimeline';
import { ConfirmDialog } from '../../../../components/ConfirmDialog';
import { DetailCard, DetailRow } from '../../../../components/DetailCard';
import { ErrorNotice } from '../../../../components/ErrorNotice';
import { NotFound } from '../../../../components/NotFound';
import { RecordAvatar } from '../../../../components/RecordAvatar';
import { ScreenFrame } from '../../../../components/ScreenFrame';
import { TextAction } from '../../../../components/TextAction';
import { toSentence } from '../../../../lib/forms';
import { recordsChanged } from '../../../../lib/records/changes';
import { contactName } from '../../../../lib/records/format';
import { useLeaveOnWorkspaceSwitch, useRecord } from '../../../../lib/records/hooks';
import { isMissing } from '../../../../lib/records/load';
import { session } from '../../../../services';
import { useTheme } from '../../../../theme';

const contacts = recordsApi(session.api, 'contacts');
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });

export default function ContactDetail() {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  useLeaveOnWorkspaceSwitch('/contacts');
  const [gone, setGone] = useState(false);
  // Once deleted, the record is not loaded again: it would answer 404 while the screen closes.
  const { state, workspaceId, reload } = useRecord(contacts.get, id, { paused: gone });
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState('');

  if (state.status === 'missing') return <NotFound title="Contact not found" backHref="/contacts" backLabel="contacts" />;

  const contact = state.record;
  const name = contact ? contactName(contact) : '';

  async function remove() {
    if (!contact || !workspaceId) return;
    setDeleting(true);
    setDeleteError('');
    try {
      await contacts.remove(workspaceId, contact.id);
    } catch (err) {
      // Already gone is what was asked for.
      if (!isMissing(err)) {
        setDeleteError(toSentence(err instanceof Error ? err.message : String(err)));
        setDeleting(false);
        return;
      }
    }
    setGone(true);
    setConfirming(false);
    setDeleting(false);
    router.dismissTo('/contacts');
    recordsChanged();
  }

  return (
    <ScreenFrame
      backLabel="Contacts"
      backHref="/contacts"
      actions={
        contact ? (
          <>
            <TextAction label="Edit" onPress={() => router.push(`/contacts/${contact.id}/edit`)} />
            <TextAction label="Delete" danger onPress={() => setConfirming(true)} />
          </>
        ) : null
      }
    >
      {state.status === 'error' ? (
        <>
          <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
            Contact
          </Text>
          <ErrorNotice message={state.error} onRetry={() => void reload()} />
        </>
      ) : !contact ? (
        <ActivityIndicator accessibilityLabel="Loading the contact" />
      ) : (
        <>
          <View style={styles.hero}>
            <RecordAvatar name={name} size={64} />
            <View style={{ flex: 1 }}>
              <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
                {name}
              </Text>
              {contact.title || contact.company_name ? (
                <Text style={{ color: theme.text2, fontSize: 16 }}>{[contact.title, contact.company_name].filter(Boolean).join(' at ')}</Text>
              ) : null}
            </View>
          </View>
          <DetailCard title="Details">
            {contact.email || contact.phone || contact.company_name ? (
              <>
                {contact.email ? <DetailRow label="Email" value={contact.email} url={`mailto:${contact.email}`} /> : null}
                {contact.phone ? <DetailRow label="Phone" value={contact.phone} url={`tel:${contact.phone}`} /> : null}
                {contact.company_name && contact.company_id ? (
                  <DetailRow label="Company" value={contact.company_name} onPress={() => router.push(`/companies/${contact.company_id}`)} />
                ) : null}
              </>
            ) : (
              <Text style={{ color: theme.text2 }}>No email, phone, or company yet.</Text>
            )}
          </DetailCard>
          {workspaceId ? <ActivityTimeline workspaceId={workspaceId} link={{ contact_id: contact.id }} /> : null}
          <Text style={{ color: theme.text2, fontSize: 13 }}>
            Added {date.format(new Date(contact.created_at))} · Updated {date.format(new Date(contact.updated_at))}
          </Text>
        </>
      )}
      <ConfirmDialog
        visible={confirming}
        title={`Delete ${name || 'this contact'}?`}
        message="They are removed from your contacts. Deals keep their history without the contact."
        confirmLabel="Delete contact"
        busy={deleting}
        error={deleteError}
        onConfirm={() => void remove()}
        onCancel={() => setConfirming(false)}
      />
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({
  hero: { flexDirection: 'row', alignItems: 'center', gap: 16 },
  title: { fontSize: 26, fontWeight: '700' }
});
