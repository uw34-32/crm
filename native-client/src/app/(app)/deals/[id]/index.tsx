import { useEffect, useState } from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { dealsApi, pipelinesApi, type Pipeline } from '../../../../api/crm';
import { ActivityTimeline } from '../../../../components/ActivityTimeline';
import { Button } from '../../../../components/Button';
import { ConfirmDialog } from '../../../../components/ConfirmDialog';
import { DetailCard, DetailRow } from '../../../../components/DetailCard';
import { ErrorNotice } from '../../../../components/ErrorNotice';
import { MoveDealSheet } from '../../../../components/MoveDealSheet';
import { NotFound } from '../../../../components/NotFound';
import { ScreenFrame } from '../../../../components/ScreenFrame';
import { TextAction } from '../../../../components/TextAction';
import { buildColumns } from '../../../../lib/deals/board';
import { formatMoney } from '../../../../lib/deals/money';
import { toSentence } from '../../../../lib/forms';
import { useMembers } from '../../../../lib/members';
import { recordsChanged } from '../../../../lib/records/changes';
import { useLeaveOnWorkspaceSwitch, useRecord } from '../../../../lib/records/hooks';
import { isMissing } from '../../../../lib/records/load';
import { memberName } from '../../../../lib/activities/timeline';
import { session } from '../../../../services';
import { useTheme } from '../../../../theme';

const deals = dealsApi(session.api);
const pipelines = pipelinesApi(session.api);
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });

export default function DealDetail() {
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  useLeaveOnWorkspaceSwitch('/pipeline');
  const [gone, setGone] = useState(false);
  const { state, workspaceId, reload } = useRecord(deals.get, id, { paused: gone });
  const deal = state.record;
  const { members } = useMembers(workspaceId);

  // The stages of the deal's pipeline, for the stage shown and the move.
  const [pipeline, setPipeline] = useState<Pipeline | null>(null);
  const [stagesError, setStagesError] = useState('');
  const pipelineId = deal?.pipeline_id as string | undefined;
  useEffect(() => {
    if (!workspaceId || !pipelineId) return;
    let live = true;
    setStagesError('');
    pipelines
      .get(workspaceId, pipelineId)
      .then((p) => live && setPipeline(p))
      .catch((err: unknown) => live && setStagesError(err instanceof Error ? err.message : String(err)));
    return () => {
      live = false;
    };
  }, [workspaceId, pipelineId]);
  const stage = pipeline?.stages?.find((s) => s.id === deal?.stage_id) ?? null;

  const [moving, setMoving] = useState(false);
  const [moveBusy, setMoveBusy] = useState(false);
  const [moveError, setMoveError] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState('');

  if (state.status === 'missing') return <NotFound title="Deal not found" backHref="/pipeline" backLabel="pipeline" />;

  async function moveTo({ stageId, lostReason }: { stageId: string; lostReason: string }) {
    if (!deal || !workspaceId) return;
    setMoving(false);
    setMoveBusy(true);
    setMoveError('');
    try {
      await deals.move(workspaceId, deal.id, lostReason ? { stage_id: stageId, lost_reason: lostReason } : { stage_id: stageId });
      recordsChanged();
    } catch (err) {
      setMoveError(toSentence(err instanceof Error ? err.message : String(err)));
    } finally {
      setMoveBusy(false);
    }
  }

  async function remove() {
    if (!deal || !workspaceId) return;
    setDeleting(true);
    setDeleteError('');
    try {
      await deals.remove(workspaceId, deal.id);
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
    router.dismissTo('/pipeline');
    recordsChanged();
  }

  return (
    <ScreenFrame
      backLabel="Pipeline"
      backHref="/pipeline"
      actions={
        deal ? (
          <>
            <TextAction label="Edit" onPress={() => router.push(`/deals/${deal.id}/edit`)} />
            <TextAction label="Delete" danger onPress={() => setConfirming(true)} />
          </>
        ) : null
      }
    >
      {state.status === 'error' ? (
        <>
          <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
            Deal
          </Text>
          <ErrorNotice message={state.error} onRetry={() => void reload()} />
        </>
      ) : !deal || !workspaceId ? (
        <ActivityIndicator accessibilityLabel="Loading the deal" />
      ) : (
        <>
          <View style={{ gap: 4 }}>
            <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
              {deal.title}
            </Text>
            {deal.value_minor > 0 ? <Text style={{ color: theme.text, fontSize: 20, fontWeight: '600' }}>{formatMoney(deal.value_minor, deal.currency)}</Text> : null}
          </View>
          <DetailCard title="Details">
            <DetailRow label="Stage" value={stage ? `${stage.name}${stage.kind === 'won' ? ' (won)' : stage.kind === 'lost' ? ' (lost)' : ''}` : stagesError ? 'Not available' : 'Loading…'} />
            {stagesError ? <ErrorNotice message={stagesError} /> : null}
            {pipeline ? <DetailRow label="Pipeline" value={pipeline.name} /> : null}
            {stage?.kind === 'lost' && deal.lost_reason ? <DetailRow label="Lost because" value={deal.lost_reason} /> : null}
            {deal.company_name && deal.company_id ? <DetailRow label="Company" value={deal.company_name} onPress={() => router.push(`/companies/${deal.company_id}`)} /> : null}
            {deal.contact_name && deal.contact_id ? <DetailRow label="Contact" value={deal.contact_name} onPress={() => router.push(`/contacts/${deal.contact_id}`)} /> : null}
            {deal.owner_id ? <DetailRow label="Owner" value={memberName(members, deal.owner_id)} /> : null}
            {deal.expected_close_date ? <DetailRow label="Expected close" value={deal.expected_close_date} /> : null}
            {moveError ? <ErrorNotice message={moveError} /> : null}
            <Button label={moveBusy ? 'Moving…' : 'Move to another stage'} variant="secondary" busy={moveBusy} disabled={!pipeline} onPress={() => setMoving(true)} />
          </DetailCard>
          <ActivityTimeline workspaceId={workspaceId} link={{ deal_id: deal.id }} />
          <Text style={{ color: theme.text2, fontSize: 13 }}>
            Added {date.format(new Date(deal.created_at))} · Updated {date.format(new Date(deal.updated_at))}
          </Text>
          {moving && pipeline ? (
            <MoveDealSheet deal={deal as never} columns={buildColumns(pipeline.stages ?? [], [deal as never])} showPlace={false} onCancel={() => setMoving(false)} onMove={(t) => void moveTo(t)} />
          ) : null}
        </>
      )}
      <ConfirmDialog
        visible={confirming}
        title={`Delete ${deal?.title ?? 'this deal'}?`}
        message="The deal is removed from the pipeline. Its activity stays in the history of its company and contact."
        confirmLabel="Delete deal"
        busy={deleting}
        error={deleteError}
        onConfirm={() => void remove()}
        onCancel={() => setConfirming(false)}
      />
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({ title: { fontSize: 26, fontWeight: '700' } });
