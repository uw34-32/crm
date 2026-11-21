// Moves a deal: pick a stage and a place in it. A lost stage asks for the reason too.

import { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { needsLostReason, type Column, type Deal } from '../lib/deals/board';
import { DEAL_LIMITS } from '../lib/deals/fields';
import { radius, useTheme } from '../theme';
import { Button } from './Button';
import { Field } from './Field';
import { OptionChips } from './OptionChips';

export type MoveTarget = { stageId: string; before: string | null; lostReason: string };

type Props = {
  deal: Deal;
  columns: Column[];
  /** Whether to ask for a place in the stage; a screen that knows only this deal sends none. */
  showPlace?: boolean;
  onMove: (target: MoveTarget) => void;
  onCancel: () => void;
};

type Place = 'start' | 'end' | 'keep';

export function MoveDealSheet({ deal, columns, showPlace = true, onMove, onCancel }: Props) {
  const theme = useTheme();
  const [stageId, setStageId] = useState(deal.stage_id);
  const [place, setPlace] = useState<Place>('keep');
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const target = columns.find((column) => column.stage.id === stageId) ?? null;
  const others = target ? target.deals.filter((d) => d.id !== deal.id) : [];
  const sameStage = stageId === deal.stage_id;
  const askReason = target ? needsLostReason(deal, target.stage) : false;

  function submit() {
    if (!target) return;
    const lostReason = reason.trim();
    if (askReason && !lostReason) return setError('Say why it was lost.');
    if (askReason && [...lostReason].length > DEAL_LIMITS.lost_reason) return setError(`Use at most ${DEAL_LIMITS.lost_reason} characters.`);
    // Staying put in the same stage is a keep; a new stage defaults to the end.
    const effective: Place = place === 'keep' && !sameStage ? 'end' : place;
    const index = effective === 'start' ? 0 : effective === 'end' ? others.length : (columns.find((c) => c.stage.id === deal.stage_id)?.deals.findIndex((d) => d.id === deal.id) ?? 0);
    const before = index >= others.length ? null : others[index].id;
    onMove({ stageId, before, lostReason });
  }

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onCancel}>
      <Pressable accessibilityLabel="Close dialog" style={styles.backdrop} onPress={onCancel}>
        <Pressable accessibilityViewIsModal style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }]} onPress={() => {}}>
          <ScrollView contentContainerStyle={{ gap: 14 }} keyboardShouldPersistTaps="handled">
            <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
              Move {deal.title}
            </Text>
            <OptionChips
              label="Stage"
              value={stageId}
              options={columns.map((c) => ({ value: c.stage.id, label: c.stage.name }))}
              onChange={(id) => {
                setStageId(id);
                setPlace('keep');
                setError('');
              }}
            />
            {showPlace ? (
              <OptionChips
                label="Place"
                value={place === 'keep' && !sameStage ? 'end' : place}
                options={[
                  ...(sameStage ? ([{ value: 'keep', label: 'Where it is' }] as const) : []),
                  { value: 'start', label: 'At the top' },
                  { value: 'end', label: 'At the bottom' }
                ]}
                onChange={setPlace}
              />
            ) : null}
            {askReason ? (
              <Field label="Why was it lost?" value={reason} onChangeText={setReason} error={error} multiline />
            ) : error ? (
              <Text style={{ color: theme.dangerText }}>{error}</Text>
            ) : null}
            <View style={{ gap: 10 }}>
              <Button label="Move deal" onPress={submit} />
              <Button label="Cancel" variant="secondary" onPress={onCancel} />
            </View>
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.5)', justifyContent: 'center', padding: 16 },
  card: { borderWidth: 1, borderRadius: radius, padding: 20, maxHeight: '90%', maxWidth: 480, width: '100%', alignSelf: 'center' },
  title: { fontSize: 20, fontWeight: '700' }
});
