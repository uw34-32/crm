// The activities linked to one deal, contact, or company, newest first and grouped by
// day: add one, edit it, complete or reopen a task, delete it.

import { useEffect, useMemo, useRef, useState } from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';
import { activitiesApi, type ActivityQuery } from '../api/crm';
import { toSentence } from '../lib/forms';
import { activityCreateBody, activityPatchBody, type ActivityDraft, type ActivityLink } from '../lib/activities/fields';
import { canEditActivity, dueState, groupByDay, memberName, type Activity, type DueState } from '../lib/activities/timeline';
import { createListStore, createPager } from '../lib/records/list';
import { useMembers } from '../lib/members';
import { useStore } from '../lib/store';
import { session, workspaces } from '../services';
import { radius, useTheme } from '../theme';
import { ActivityForm } from './ActivityForm';
import { Button } from './Button';
import { ConfirmDialog } from './ConfirmDialog';
import { ErrorNotice } from './ErrorNotice';
import { TextAction } from './TextAction';
import { dateLocale, t } from '../i18n';

const activities = activitiesApi(session.api);
const kindLabel = (kind: 'note' | 'call' | 'meeting' | 'task'): string => ({ note: t('Note'), call: t('Call'), meeting: t('Meeting'), task: t('Task') })[kind];
const formatTime = (value: Date) => new Intl.DateTimeFormat(dateLocale(), { dateStyle: 'medium', timeStyle: 'short' }).format(value);

const dueText = (state: DueState): string => ({ none: '', open: t('Open'), completed: t('Done'), overdue: t('Overdue'), today: t('Due today'), upcoming: t('Upcoming') })[state];

type Props = { workspaceId: string; link: ActivityLink };

export function ActivityTimeline({ workspaceId, link }: Props) {
  const theme = useTheme();
  const filter = useMemo(() => link, [JSON.stringify(link)]); // eslint-disable-line react-hooks/exhaustive-deps
  const latest = useRef({ workspaceId, filter });
  latest.current = { workspaceId, filter };
  const [{ store, pager }] = useState(() => {
    const listStore = createListStore<Activity>();
    const listPager = createPager(listStore, ({ limit, offset }) => activities.list(latest.current.workspaceId, { ...(latest.current.filter as Partial<ActivityQuery>), limit, offset }));
    return { store: listStore, pager: listPager };
  });
  const list = useStore(store);
  const { members } = useMembers(workspaceId);
  const { current } = useStore(workspaces.state);
  const user = useStore(session.state).user;

  useEffect(() => {
    void pager.load('');
    return () => pager.stop();
  }, [pager, workspaceId, filter]);

  const [formFor, setFormFor] = useState<Activity | 'new' | null>(null);
  const [formBusy, setFormBusy] = useState(false);
  const [formError, setFormError] = useState('');
  const [deleting, setDeleting] = useState<Activity | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState('');
  const [rowError, setRowError] = useState('');
  const [working, setWorking] = useState<string | null>(null);

  const refresh = () => pager.load('', { quiet: true });
  const now = new Date();
  const groups = groupByDay(list.items, now, dateLocale());
  const actor = { userId: user?.id ?? '', role: current?.role ?? 'member' };

  async function save(draft: ActivityDraft) {
    setFormBusy(true);
    setFormError('');
    try {
      if (formFor === 'new') await activities.create(workspaceId, activityCreateBody(draft, link));
      else if (formFor) {
        const body = activityPatchBody(formFor, draft);
        if (Object.keys(body).length > 0) await activities.update(workspaceId, formFor.id, body);
      }
      setFormFor(null);
      await refresh();
    } catch (err) {
      setFormError(err instanceof Error ? err.message : String(err));
    } finally {
      setFormBusy(false);
    }
  }

  async function toggle(activity: Activity) {
    setWorking(activity.id);
    setRowError('');
    try {
      if (activity.completed_at) await activities.reopen(workspaceId, activity.id);
      else await activities.complete(workspaceId, activity.id);
      await refresh();
    } catch (err) {
      setRowError(toSentence(err instanceof Error ? err.message : String(err)));
    } finally {
      setWorking(null);
    }
  }

  async function remove() {
    if (!deleting) return;
    setDeleteBusy(true);
    setDeleteError('');
    try {
      await activities.remove(workspaceId, deleting.id);
      setDeleting(null);
      await refresh();
    } catch (err) {
      setDeleteError(toSentence(err instanceof Error ? err.message : String(err)));
    } finally {
      setDeleteBusy(false);
    }
  }

  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }]}>
      <Text accessibilityRole="header" style={[styles.heading, { color: theme.text }]}>
        {t('Activity')}
      </Text>
      <Button label={t('Add activity')} variant="secondary" onPress={() => { setFormError(''); setFormFor('new'); }} />
      {rowError ? <ErrorNotice message={rowError} /> : null}
      {list.status === 'error' ? (
        <ErrorNotice message={list.error} onRetry={() => void pager.load('')} />
      ) : list.status !== 'ready' ? (
        <ActivityIndicator accessibilityLabel={t('Loading activity')} />
      ) : list.items.length === 0 ? (
        <Text style={{ color: theme.text2 }}>{t('Nothing yet. Add a note, a call, a meeting, or a task.')}</Text>
      ) : (
        groups.map((group) => (
          <View key={group.key} style={{ gap: 8 }}>
            <Text style={{ color: theme.text2, fontSize: 13, fontWeight: '600' }}>{group.label}</Text>
            {group.items.map((activity) => {
              const state = dueState(activity, now);
              const mayEdit = canEditActivity(activity, actor);
              return (
                <View key={activity.id} style={[styles.item, { backgroundColor: theme.bg, borderColor: state === 'overdue' ? theme.danger : theme.border }]}>
                  <Text style={{ color: theme.text2, fontSize: 13 }}>
                    {kindLabel(activity.kind)}
                    {state !== 'none' ? ` · ${dueText(state)}` : ''}
                    {activity.due_at ? ` · ${formatTime(new Date(activity.due_at))}` : ''}
                  </Text>
                  <Text style={{ color: theme.text, fontSize: 16, fontWeight: '600', textDecorationLine: activity.completed_at ? 'line-through' : 'none' }}>{activity.subject}</Text>
                  {activity.body ? <Text style={{ color: theme.text }}>{activity.body}</Text> : null}
                  <Text style={{ color: theme.text2, fontSize: 13 }}>
                    {memberName(members, activity.author_id)}
                    {activity.assignee_id ? ` · ${t('for {name}', { name: memberName(members, activity.assignee_id) })}` : ''}
                  </Text>
                  <View style={styles.actions}>
                    {activity.kind === 'task' ? <TextAction label={working === activity.id ? t('Working…') : activity.completed_at ? t('Reopen') : t('Complete')} onPress={() => void toggle(activity)} /> : null}
                    {mayEdit ? <TextAction label={t('Edit')} onPress={() => { setFormError(''); setFormFor(activity); }} /> : null}
                    {mayEdit ? <TextAction label={t('Delete')} danger onPress={() => { setDeleteError(''); setDeleting(activity); }} /> : null}
                  </View>
                </View>
              );
            })}
          </View>
        ))
      )}
      {list.status === 'ready' && list.hasMore ? <Button label={list.loadingMore ? t('Loading…') : t('Show older activity')} variant="secondary" busy={list.loadingMore} onPress={() => void pager.more()} /> : null}
      {list.moreError ? <ErrorNotice message={list.moreError} onRetry={() => void pager.more()} /> : null}
      {formFor ? <ActivityForm workspaceId={workspaceId} activity={formFor === 'new' ? null : formFor} busy={formBusy} error={formError} onSubmit={(d) => void save(d)} onCancel={() => setFormFor(null)} /> : null}
      <ConfirmDialog
        visible={deleting !== null}
        title={t('Delete this activity?')}
        message={deleting ? t('“{name}” is removed from the timeline.', { name: deleting.subject }) : ''}
        confirmLabel={t('Delete activity')}
        busy={deleteBusy}
        error={deleteError}
        onConfirm={() => void remove()}
        onCancel={() => setDeleting(null)}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { borderWidth: 1, borderRadius: radius, padding: 16, gap: 12 },
  heading: { fontSize: 17, fontWeight: '600' },
  item: { borderWidth: 1, borderRadius: radius, padding: 12, gap: 4 },
  actions: { flexDirection: 'row', gap: 16, flexWrap: 'wrap' }
});
