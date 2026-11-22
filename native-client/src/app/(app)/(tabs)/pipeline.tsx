import { useEffect, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { dealsApi, pipelinesApi } from '../../../api/crm';
import { Button } from '../../../components/Button';
import { DealCard } from '../../../components/DealCard';
import { ErrorNotice } from '../../../components/ErrorNotice';
import { MoveDealSheet } from '../../../components/MoveDealSheet';
import { OptionChips } from '../../../components/OptionChips';
import { stageTotals } from '../../../lib/deals/board';
import { createBoard } from '../../../lib/deals/boardState';
import { formatMoney } from '../../../lib/deals/money';
import { useWorkspaceId } from '../../../lib/records/hooks';
import { changes } from '../../../lib/records/changes';
import { useStore } from '../../../lib/store';
import { session } from '../../../services';
import { radius, useTheme } from '../../../theme';
import { nameOf, t, useLocale } from '../../../i18n';

const board = createBoard({ pipelines: pipelinesApi(session.api), deals: dealsApi(session.api) });

export default function Pipeline() {
  useLocale();
  const theme = useTheme();
  const workspaceId = useWorkspaceId();
  const { version } = useStore(changes);
  const state = useStore(board.store);
  const [stageId, setStageId] = useState('');
  const [moving, setMoving] = useState<string | null>(null);

  // A new workspace or pipeline starts over; a change to some deal refreshes behind the board.
  const [loaded, setLoaded] = useState({ version, workspaceId: '' });
  useEffect(() => {
    if (!workspaceId) return;
    const quiet = loaded.workspaceId === workspaceId && loaded.version !== version;
    setLoaded({ version, workspaceId });
    void board.load(workspaceId, state.pipeline?.id ?? '', { quiet });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, version]);
  useEffect(() => () => board.stop(), []);

  const columns = state.columns;
  const column = columns.find((c) => c.stage.id === stageId) ?? columns[0] ?? null;
  const movingDeal = moving ? columns.flatMap((c) => c.deals).find((d) => d.id === moving) : null;
  const closedKind = column && column.stage.kind !== 'open' ? column.stage.kind : null;
  const paging = closedKind ? state.closed[closedKind] : null;

  const header = (
    <View style={styles.header}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {t('Pipeline')}
      </Text>
      {state.pipelines.length > 1 ? (
        <OptionChips
          label={t('Pipeline')}
          value={state.pipeline?.id ?? ''}
          options={state.pipelines.map((p) => ({ value: p.id, label: nameOf(p.name) }))}
          onChange={(id) => {
            setStageId('');
            if (workspaceId) void board.load(workspaceId, id);
          }}
        />
      ) : null}
      {state.status === 'ready' && columns.length > 0 ? (
        <View style={styles.stages} accessibilityRole="tablist">
          {columns.map((c) => {
            const selected = c.stage.id === column?.stage.id;
            const totals = stageTotals(c.deals);
            return (
              <Pressable
                key={c.stage.id}
                accessibilityRole="tab"
                accessibilityState={{ selected }}
                onPress={() => setStageId(c.stage.id)}
                style={[styles.stage, { backgroundColor: selected ? theme.accentStrong : theme.surface, borderColor: selected ? theme.accentStrong : theme.border }]}
              >
                <Text style={{ color: selected ? '#fff' : theme.text, fontWeight: '600' }}>
                  {nameOf(c.stage.name)} · {c.deals.length}
                  {c.hasMore ? '+' : ''}
                </Text>
                {totals[0] ? (
                  <Text style={{ color: selected ? '#fff' : theme.text2, fontSize: 12 }}>
                    {formatMoney(totals[0].minor, totals[0].currency, undefined, { compact: true })}
                    {totals.length > 1 ? ` +${totals.length - 1}` : ''}
                  </Text>
                ) : null}
              </Pressable>
            );
          })}
        </View>
      ) : null}
      {state.notice ? <ErrorNotice message={state.notice} /> : null}
      {state.truncated ? <Text style={{ color: theme.text2 }}>{t('Only the first open deals are shown.')}</Text> : null}
    </View>
  );

  let empty = null;
  if (state.status === 'error') empty = <ErrorNotice message={state.error} onRetry={() => workspaceId && void board.load(workspaceId, state.pipeline?.id ?? '')} />;
  else if (state.status === 'loading') empty = <ActivityIndicator accessibilityLabel={t('Loading the pipeline')} style={{ marginVertical: 24 }} />;
  else if (!state.pipeline) empty = <Text style={{ color: theme.text2, padding: 24, textAlign: 'center' }}>{t('This workspace has no pipeline yet.')}</Text>;
  else if (column && column.deals.length === 0) empty = <Text style={{ color: theme.text2, padding: 24, textAlign: 'center' }}>{t('No deals in {name}.', { name: nameOf(column.stage.name) })}</Text>;

  const footer =
    closedKind && paging?.hasMore ? (
      <View style={{ gap: 8 }}>
        {paging.error ? <ErrorNotice message={paging.error} /> : null}
        <Button label={paging.loading ? t('Loading…') : closedKind === 'won' ? t('Show more won deals') : t('Show more lost deals')} variant="secondary" busy={paging.loading} onPress={() => void board.loadMore(closedKind)} />
      </View>
    ) : null;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }} edges={['top', 'left', 'right']}>
      <FlatList
        data={state.status === 'ready' && column ? column.deals : []}
        keyExtractor={(deal) => deal.id}
        contentContainerStyle={styles.content}
        ListHeaderComponent={header}
        ListEmptyComponent={empty ?? undefined}
        ListFooterComponent={footer ?? undefined}
        renderItem={({ item }) => <DealCard deal={item} busy={Boolean(state.busy[item.id])} onMove={() => setMoving(item.id)} />}
      />
      {/* Recording a sale is what this tab is for, so its button stays in reach however far the list is scrolled. */}
      {state.status === 'ready' && state.pipeline ? (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('Record sale')}
        onPress={() => router.push(state.pipeline ? `/deals/new?pipeline=${state.pipeline.id}` : '/deals/new')}
        style={[styles.record, { backgroundColor: theme.accentStrong }]}
      >
        <Text style={styles.recordLabel}>+ {t('Record sale')}</Text>
      </Pressable>
      ) : null}
      {movingDeal ? (
        <MoveDealSheet
          deal={movingDeal}
          columns={columns}
          onCancel={() => setMoving(null)}
          onMove={({ stageId: to, before, lostReason }) => {
            setMoving(null);
            void board.move(movingDeal.id, to, before, lostReason);
          }}
        />
      ) : null}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  // Room under the list for the fixed button.
  content: { padding: 16, paddingBottom: 96, gap: 10, maxWidth: 640, width: '100%', alignSelf: 'center' },
  record: { position: 'absolute', right: 16, bottom: 16, minHeight: 48, paddingHorizontal: 20, borderRadius: 24, alignItems: 'center', justifyContent: 'center', elevation: 4, shadowColor: '#000', shadowOpacity: 0.25, shadowRadius: 6, shadowOffset: { width: 0, height: 2 } },
  recordLabel: { color: '#fff', fontSize: 16, fontWeight: '700' },
  header: { gap: 12, marginBottom: 6 },
  title: { fontSize: 28, fontWeight: '700' },
  stages: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  stage: { borderWidth: 1, borderRadius: radius, paddingHorizontal: 12, paddingVertical: 8, minHeight: 48, justifyContent: 'center' }
});
