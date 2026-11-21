import { Pressable, Text } from 'react-native';
import { useTheme } from '../theme';

export function TextAction({ label, onPress, danger = false }: { label: string; onPress: () => void; danger?: boolean }) {
  const theme = useTheme();
  return (
    <Pressable accessibilityRole="button" hitSlop={8} onPress={onPress} style={{ minHeight: 44, justifyContent: 'center' }}>
      <Text style={{ color: danger ? theme.dangerText : theme.accentText, fontSize: 16, fontWeight: '600' }}>{label}</Text>
    </Pressable>
  );
}
