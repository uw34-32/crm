// Adds or edits an activity in a dialog. Only a task has a due time and an assignee, and
// a kind never changes once the activity exists.

import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { toSentence } from '../lib/forms';
import { ACTIVITY_KINDS, validateActivity, toLocalInput, type ActivityDraft, type ActivityKind } from '../lib/activities/fields';
import type { Activity } from '../lib/activities/timeline';
import { radius, useTheme } from '../theme';
import { Button } from './Button';
import { ErrorNotice } from './ErrorNotice';
import { Field } from './Field';
import { MemberPicker } from './MemberPicker';
import { OptionChips } from './OptionChips';
import { t } from '../i18n';

const kindLabel = (kind: ActivityKind): string => ({ note: t('Note'), call: t('Call'), meeting: t('Meeting'), task: t('Task') })[kind];

type Props = {
  workspaceId: string;
  /** The activity being edited; none when adding. */
  activity?: Activity | null;
  busy: boolean;
  error: string;
  onSubmit: (draft: ActivityDraft) => void;
  onCancel: () => void;
};

export function ActivityForm({ workspaceId, activity = null, busy, error, onSubmit, onCancel }: Props) {
  const theme = useTheme();
  const [kind, setKind] = useState<ActivityKind>(activity?.kind ?? 'note');
  const [subject, setSubject] = useState(activity?.subject ?? '');
  const [body, setBody] = useState(activity?.body ?? '');
  // A time is typed as 2027-03-14 09:30, in the device's time zone.
  const [due, setDue] = useState(toLocalInput(activity?.due_at).replace('T', ' '));
  const [assignee, setAssignee] = useState<string>(activity?.assignee_id ?? '');
  const [errors, setErrors] = useState<Record<string, string>>({});

  function submit() {
    const draft: ActivityDraft = { kind, subject, body, due_at: due.trim().replace(' ', 'T'), assignee_id: assignee || null };
    const found = validateActivity(draft);
    setErrors(found);
    if (Object.keys(found).length === 0) onSubmit(draft);
  }

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onCancel}>
      <Pressable accessibilityLabel={t('Close dialog')} style={styles.backdrop} onPress={busy ? undefined : onCancel}>
        <Pressable accessibilityViewIsModal style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }]} onPress={() => {}}>
          <ScrollView contentContainerStyle={{ gap: 14 }} keyboardShouldPersistTaps="handled">
            <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
              {activity ? t('Edit activity') : t('Add activity')}
            </Text>
            {error ? <ErrorNotice message={toSentence(error)} /> : null}
            <OptionChips
              label={t('Kind')}
              value={kind}
              disabled={Boolean(activity)}
              options={ACTIVITY_KINDS.map((k) => ({ value: k, label: kindLabel(k) }))}
              onChange={setKind}
            />
            <Field label={t('Subject')} value={subject} onChangeText={setSubject} error={errors.subject} />
            <Field label={t('Notes')} value={body} onChangeText={setBody} error={errors.body} multiline />
            {kind === 'task' ? (
              <>
                <Field label={t('Due')} value={due} onChangeText={setDue} error={errors.due_at} placeholder="2027-03-14 09:30" autoCapitalize="none" autoCorrect={false} />
                <MemberPicker label={t('Assignee')} workspaceId={workspaceId} value={assignee} onChange={setAssignee} emptyLabel={t('Nobody')} />
              </>
            ) : null}
            <View style={{ gap: 10 }}>
              <Button label={activity ? t('Save changes') : t('Add activity')} busy={busy} onPress={submit} />
              <Button label={t('Cancel')} variant="secondary" disabled={busy} onPress={onCancel} />
            </View>
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.5)', justifyContent: 'center', padding: 16 },
  card: { borderWidth: 1, borderRadius: radius, padding: 20, maxHeight: '92%', maxWidth: 480, width: '100%', alignSelf: 'center' },
  title: { fontSize: 20, fontWeight: '700' }
});
