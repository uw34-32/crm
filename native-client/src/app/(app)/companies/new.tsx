import { useState } from 'react';
import { StyleSheet, Text } from 'react-native';
import { router } from 'expo-router';
import { recordsApi } from '../../../api/records';
import { CompanyForm } from '../../../components/CompanyForm';
import { ScreenFrame } from '../../../components/ScreenFrame';
import { companyCreateBody, type CompanyDraft } from '../../../lib/records/fields';
import { recordsChanged } from '../../../lib/records/changes';
import { useLeaveOnWorkspaceSwitch } from '../../../lib/records/hooks';
import { session } from '../../../services';
import { useTheme } from '../../../theme';
import { t } from '../../../i18n';

const companies = recordsApi(session.api, 'companies');

export default function NewCompany() {
  const theme = useTheme();
  const workspaceId = useLeaveOnWorkspaceSwitch('/companies');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function create(draft: CompanyDraft) {
    if (!workspaceId) return;
    setBusy(true);
    setError('');
    try {
      const company = await companies.create(workspaceId, companyCreateBody(draft));
      recordsChanged();
      router.replace(`/companies/${company.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScreenFrame backLabel={t('Companies')} backHref="/companies">
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {t('New company')}
      </Text>
      <CompanyForm submitLabel={t('Add company')} busy={busy} error={error} onSubmit={(d) => void create(d)} onCancel={() => router.back()} />
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({ title: { fontSize: 28, fontWeight: '700' } });
