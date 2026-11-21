// Picks a member of the workspace, or nobody.

import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useMembers } from '../lib/members';
import { radius, useTheme } from '../theme';

type Props = { label: string; workspaceId: string; value: string; onChange: (userId: string) => void; emptyLabel?: string };

export function MemberPicker({ label, workspaceId, value, onChange, emptyLabel = 'Nobody' }: Props) {
  const theme = useTheme();
  const { members, status, retry } = useMembers(workspaceId);
  const options = [{ id: '', name: emptyLabel }, ...members.map((m) => ({ id: m.user_id, name: m.name?.trim() || m.email }))];
  return (
    <View style={styles.group} accessibilityRole="radiogroup" accessibilityLabel={label}>
      <Text style={[styles.label, { color: theme.text }]}>{label}</Text>
      {status === 'error' ? (
        <Pressable accessibilityRole="button" onPress={() => void retry()}>
          <Text style={{ color: theme.dangerText }}>The members could not be loaded. Tap to try again.</Text>
        </Pressable>
      ) : null}
      <View style={styles.chips}>
        {options.map((option) => {
          const selected = option.id === value;
          return (
            <Pressable
              key={option.id || 'none'}
              accessibilityRole="radio"
              accessibilityState={{ checked: selected }}
              onPress={() => onChange(option.id)}
              style={[styles.chip, { backgroundColor: selected ? theme.accentStrong : theme.surface2, borderColor: selected ? theme.accentStrong : theme.border }]}
            >
              <Text style={{ color: selected ? '#fff' : theme.text }}>{option.name}</Text>
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  group: { gap: 8 },
  label: { fontSize: 14, fontWeight: '600' },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: { borderWidth: 1, borderRadius: radius, paddingHorizontal: 12, minHeight: 40, justifyContent: 'center' }
});
