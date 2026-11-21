import { Pressable, StyleSheet, Text, View } from 'react-native';
import { Link } from 'expo-router';
import { useStore } from '../lib/store';
import { workspaces } from '../services';
import { radius, useTheme } from '../theme';

/** Every workspace the user belongs to, with the open one marked. */
export function WorkspaceSwitcher() {
  const theme = useTheme();
  const { list, current } = useStore(workspaces.state);
  return (
    <View accessibilityRole="radiogroup" accessibilityLabel="Workspaces" style={styles.list}>
      {list.map((workspace) => {
        const selected = workspace.id === current?.id;
        return (
          <Pressable
            key={workspace.id}
            accessibilityRole="radio"
            accessibilityState={{ checked: selected }}
            onPress={() => workspaces.select(workspace.id)}
            style={[styles.row, { backgroundColor: theme.surface, borderColor: selected ? theme.accent : theme.border }]}
          >
            <Text style={[styles.name, { color: theme.text }]}>{workspace.name}</Text>
            <Text style={{ color: theme.text2 }}>{selected ? `${workspace.role} · open` : workspace.role}</Text>
          </Pressable>
        );
      })}
      <Link href="/workspaces/new" style={{ color: theme.accentText, paddingVertical: 8 }}>
        New workspace
      </Link>
    </View>
  );
}

const styles = StyleSheet.create({
  list: { gap: 8 },
  row: { borderWidth: 2, borderRadius: radius, padding: 14, gap: 2, minHeight: 56, justifyContent: 'center' },
  name: { fontSize: 16, fontWeight: '600' }
});
