import { useState } from 'react';
import { ActivityIndicator, StyleSheet, Text } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { dealsApi } from '../../../../api/crm';
import { DealForm } from '../../../../components/DealForm';
import { ErrorNotice } from '../../../../components/ErrorNotice';
import { NotFound } from '../../../../components/NotFound';
import { ScreenFrame } from '../../../../components/ScreenFrame';
import { dealPatchBody, type DealDraft } from '../../../../lib/deals/fields';
import { recordsChanged } from '../../../../lib/records/changes';
import { useRecord, useWorkspaceId } from '../../../../lib/records/hooks';
import { isMissing } from '../../../../lib/records/load';
import { session } from '../../../../services';
import { useTheme } from '../../../../theme';

const deals = dealsApi(session.api);

export default function EditDeal() {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { state, reload } = useRecord(deals.get, id);
  const workspaceId = useWorkspaceId();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  if (state.status === 'missing') return <NotFound title="Deal not found" backHref="/pipeline" backLabel="pipeline" />;
  const deal = state.record;

  async function save(draft: DealDraft) {
    if (!deal || !workspaceId) return;
    const body = dealPatchBody(deal, draft);
    if (Object.keys(body).length === 0) return router.back();
    setBusy(true);
    setError('');
    try {
      await deals.update(workspaceId, deal.id, body);
      recordsChanged();
      router.back();
    } catch (err) {
      setError(isMissing(err) ? 'This deal, or the chosen company, contact, or owner, no longer exists.' : err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScreenFrame backLabel="Deal" backHref={`/deals/${id}`}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        Edit deal
      </Text>
      {state.status === 'error' ? (
        <ErrorNotice message={state.error} onRetry={() => void reload()} />
      ) : !deal || !workspaceId ? (
        <ActivityIndicator accessibilityLabel="Loading the deal" />
      ) : (
        <DealForm
          workspaceId={workspaceId}
          initial={{
            title: deal.title,
            currency: deal.currency,
            value_minor: deal.value_minor,
            pipeline_id: deal.pipeline_id,
            expected_close_date: deal.expected_close_date,
            owner_id: deal.owner_id,
            company: deal.company_id ? { id: deal.company_id, name: deal.company_name || 'Linked company' } : null,
            contact: deal.contact_id ? { id: deal.contact_id, name: deal.contact_name || 'Linked contact' } : null
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
