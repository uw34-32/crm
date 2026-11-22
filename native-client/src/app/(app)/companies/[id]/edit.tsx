import { useState } from 'react';
import { ActivityIndicator, StyleSheet, Text } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { recordsApi } from '../../../../api/records';
import { CompanyForm } from '../../../../components/CompanyForm';
import { ErrorNotice } from '../../../../components/ErrorNotice';
import { NotFound } from '../../../../components/NotFound';
import { ScreenFrame } from '../../../../components/ScreenFrame';
import { recordsChanged } from '../../../../lib/records/changes';
import { companyPatchBody, type CompanyDraft } from '../../../../lib/records/fields';
import { useLeaveOnWorkspaceSwitch, useRecord } from '../../../../lib/records/hooks';
import { isMissing } from '../../../../lib/records/load';
import { session } from '../../../../services';
import { useTheme } from '../../../../theme';
import { t, useLocale } from '../../../../i18n';

const companies = recordsApi(session.api, 'companies');

export default function EditCompany() {
  useLocale();
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  useLeaveOnWorkspaceSwitch('/companies');
  const { state, workspaceId, reload } = useRecord(companies.get, id);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  if (state.status === 'missing') return <NotFound title={t('Company not found')} backHref="/companies" backLabel={t('companies')} />;
  const company = state.record;

  async function save(draft: CompanyDraft) {
    if (!company || !workspaceId) return;
    const body = companyPatchBody(company, draft);
    if (Object.keys(body).length === 0) return router.back();
    setBusy(true);
    setError('');
    try {
      await companies.update(workspaceId, company.id, body);
      recordsChanged();
      router.back();
    } catch (err) {
      setError(isMissing(err) ? t('This company no longer exists. Go back and check the list.') : err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScreenFrame backLabel={t('Company')} backHref={`/companies/${id}`}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {t('Edit company')}
      </Text>
      {state.status === 'error' ? (
        <ErrorNotice message={state.error} onRetry={() => void reload()} />
      ) : !company ? (
        <ActivityIndicator accessibilityLabel={t('Loading the company')} />
      ) : (
        <CompanyForm
          initial={{ name: company.name, domain: company.domain ?? '', phone: company.phone ?? '', industry: company.industry ?? '', address: company.address ?? '' }}
          submitLabel={t('Save changes')}
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
