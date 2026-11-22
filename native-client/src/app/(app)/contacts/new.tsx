import { useEffect, useState } from 'react';
import { ActivityIndicator, StyleSheet, Text } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { recordsApi, type Company } from '../../../api/records';
import { Button } from '../../../components/Button';
import { ErrorNotice } from '../../../components/ErrorNotice';
import { ContactForm } from '../../../components/ContactForm';
import { ScreenFrame } from '../../../components/ScreenFrame';
import type { Choice } from '../../../components/RecordPicker';
import { contactCreateBody, type ContactDraft } from '../../../lib/records/fields';
import { recordsChanged } from '../../../lib/records/changes';
import { useLeaveOnWorkspaceSwitch } from '../../../lib/records/hooks';
import { isMissing } from '../../../lib/records/load';
import { session } from '../../../services';
import { useTheme } from '../../../theme';
import { t, useLocale } from '../../../i18n';

const contacts = recordsApi(session.api, 'contacts');
const companies = recordsApi(session.api, 'companies');

export default function NewContact() {
  useLocale();
  const theme = useTheme();
  const workspaceId = useLeaveOnWorkspaceSwitch('/contacts');
  // Arriving from a company's screen links the new contact to it.
  const { company: companyId } = useLocalSearchParams<{ company?: string }>();
  const [preset, setPreset] = useState<Choice | null | undefined>(companyId ? undefined : null);
  const [presetError, setPresetError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!companyId || !workspaceId) return;
    let live = true;
    setPresetError('');
    companies
      .get(workspaceId, companyId)
      .then((company: Company) => live && setPreset({ id: company.id, name: company.name }))
      .catch((err: unknown) => {
        if (!live) return;
        // A company that is gone is not an error to retry: the contact can still be added.
        if (isMissing(err)) {
          setPreset(null);
          setPresetError(t('That company no longer exists, so the new contact will not be linked to it.'));
        } else setPresetError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      live = false;
    };
  }, [companyId, workspaceId, attempt]);

  async function create(draft: ContactDraft) {
    if (!workspaceId) return;
    setBusy(true);
    setError('');
    try {
      const contact = await contacts.create(workspaceId, contactCreateBody(draft));
      recordsChanged();
      router.replace(`/contacts/${contact.id}`);
    } catch (err) {
      setError(isMissing(err) ? t('The chosen company no longer exists. Choose another one, or remove the link.') : err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScreenFrame backLabel={t('Contacts')} backHref="/contacts">
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {t('New contact')}
      </Text>
      {preset === undefined && presetError ? (
        <>
          <ErrorNotice message={`${t('The company could not be loaded.')} ${presetError}`} onRetry={() => setAttempt((n) => n + 1)} />
          <Button label={t('Add without the company')} variant="secondary" onPress={() => setPreset(null)} />
        </>
      ) : preset === undefined || !workspaceId ? (
        <ActivityIndicator accessibilityLabel={t('Loading the company')} />
      ) : (
        <>
          {presetError ? <ErrorNotice message={presetError} /> : null}
          <ContactForm workspaceId={workspaceId} initial={{ company: preset }} submitLabel={t('Add contact')} busy={busy} error={error} onSubmit={(d) => void create(d)} onCancel={() => router.back()} />
        </>
      )}
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({ title: { fontSize: 28, fontWeight: '700' } });
