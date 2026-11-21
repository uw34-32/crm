// A small single choice: a row of chips.

import { Pressable, StyleSheet, Text, View } from 'react-native';
import { radius, useTheme } from '../theme';

type Props<V extends string> = { label: string; value: V; options: readonly { value: V; label: string }[]; onChange: (value: V) => void; disabled?: boolean };

export function OptionChips<V extends string>({ label, value, options, onChange, disabled = false }: Props<V>) {
  const theme = useTheme();
  return (
    <View style={styles.group} accessibilityRole="radiogroup" accessibilityLabel={label}>
      <Text style={[styles.label, { color: theme.text }]}>{label}</Text>
      <View style={styles.chips}>
        {options.map((option) => {
          const selected = option.value === value;
          return (
            <Pressable
              key={option.value}
              accessibilityRole="radio"
              accessibilityState={{ checked: selected, disabled }}
              disabled={disabled}
              onPress={() => onChange(option.value)}
              style={[styles.chip, { backgroundColor: selected ? theme.accentStrong : theme.surface2, borderColor: selected ? theme.accentStrong : theme.border, opacity: disabled ? 0.6 : 1 }]}
            >
              <Text style={{ color: selected ? '#fff' : theme.text }}>{option.label}</Text>
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
