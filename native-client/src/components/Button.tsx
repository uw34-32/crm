import { ActivityIndicator, Pressable, StyleSheet, Text } from 'react-native';
import { radius, useTheme } from '../theme';

type Props = {
  label: string;
  onPress: () => void;
  busy?: boolean;
  disabled?: boolean;
  variant?: 'primary' | 'secondary' | 'danger';
};

export function Button({ label, onPress, busy = false, disabled = false, variant = 'primary' }: Props) {
  const theme = useTheme();
  const primary = variant !== 'secondary';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: disabled || busy, busy }}
      disabled={disabled || busy}
      onPress={onPress}
      style={({ pressed }) => [
        styles.button,
        { backgroundColor: variant === 'danger' ? theme.dangerStrong : primary ? theme.accentStrong : theme.surface2, opacity: disabled || busy ? 0.6 : pressed ? 0.85 : 1 }
      ]}
    >
      {busy ? <ActivityIndicator color={primary ? '#fff' : theme.text} /> : null}
      <Text style={[styles.label, { color: primary ? '#fff' : theme.text }]}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: { minHeight: 48, borderRadius: radius, paddingHorizontal: 16, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 8 },
  label: { fontSize: 16, fontWeight: '600' }
});
