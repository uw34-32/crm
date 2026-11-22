import { useEffect, useState } from 'react';
import { ActivityIndicator, StyleSheet, Text } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { dealsApi, pipelinesApi } from '../../../api/crm';
import { DealForm } from '../../../components/DealForm';
import { ErrorNotice } from '../../../components/ErrorNotice';
import { ScreenFrame } from '../../../components/ScreenFrame';
import { dealCreateBody, type DealDraft } from '../../../lib/deals/fields';
import { markWon, wonStageId } from '../../../lib/deals/won';
import { setPendingNotice } from '../../../lib/records/notice';
import { toSentence } from '../../../lib/forms';
import { recordsChanged } from '../../../lib/records/changes';
import { useWorkspaceId } from '../../../lib/records/hooks';
import { isMissing } from '../../../lib/records/load';
import { session } from '../../../services';
import { useTheme } from '../../../theme';
import { t, useLocale } from '../../../i18n';

const deals = dealsApi(session.api);
const pipelines = pipelinesApi(session.api);

export default function NewDeal() {
  useLocale();
  const theme = useTheme();
  const workspaceId = useWorkspaceId();
  // The board links here with the pipeline it showed, so the deal starts there.
  const { pipeline: wanted } = useLocalSearchParams<{ pipeline?: string }>();
  const [choices, setChoices] = useState<{ id: string; name: string }[] | null>(null);
  const [loadError, setLoadError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!workspaceId) return;
    let live = true;
    setLoadError('');
    pipelines
      .list(workspaceId)
      .then((result) => live && setChoices([...result.items].sort((a, b) => Number(b.id === wanted) - Number(a.id === wanted))))
      .catch((err: unknown) => live && setLoadError(err instanceof Error ? err.message : String(err)));
    return () => {
      live = false;
    };
  }, [workspaceId, wanted, attempt]);

  async function create(draft: DealDraft) {
    if (!workspaceId) return;
    setBusy(true);
    setError('');
    try {
      // A sale to be closed needs a won stage, so find it first and create nothing if there is none.
      const wonId = draft.status === 'won' ? await wonStageId(pipelines, workspaceId, draft.pipeline_id ?? '') : '';
      const deal = await deals.create(workspaceId, dealCreateBody(draft));
      if (wonId) {
        // The API starts a deal in an open stage; the sale is closed by moving it. The deal exists
        // either way, so a failed move is explained on the deal's own page, once.
        await markWon(deals, workspaceId, deal.id, wonId).catch((err: unknown) =>
          setPendingNotice(`${t('Added, but it could not be marked as won. Move it to Won from here.')} ${toSentence(err instanceof Error ? err.message : String(err))}`)
        );
      }
      recordsChanged();
      router.replace(`/deals/${deal.id}`);
    } catch (err) {
      setError(isMissing(err) ? t('The pipeline, stage, company, contact, or owner no longer exists. Choose another one.') : err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScreenFrame backLabel={t('Pipeline')} backHref="/pipeline">
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {t('Record a sale')}
      </Text>
      {loadError ? (
        <ErrorNotice message={loadError} onRetry={() => setAttempt((n) => n + 1)} />
      ) : !choices || !workspaceId ? (
        <ActivityIndicator accessibilityLabel={t('Loading the pipelines')} />
      ) : (
        <DealForm workspaceId={workspaceId} pipelines={choices} creating submitLabel={t('Record sale')} busy={busy} error={error} onSubmit={(d) => void create(d)} onCancel={() => router.back()} />
      )}
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({ title: { fontSize: 28, fontWeight: '700' } });
