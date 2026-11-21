import type { ComponentRef, Ref } from 'react';
import { StyleSheet, Text, TextInput, View, type TextInputProps } from 'react-native';
import { radius, useTheme } from '../theme';

export type TextInputHandle = ComponentRef<typeof TextInput>;

type Props = Omit<TextInputProps, 'style' | 'ref'> & { label: string; error?: string; ref?: Ref<TextInputHandle> };

export function Field({ label, error, ref, ...input }: Props) {
  const theme = useTheme();
  return (
    <View style={styles.field}>
      <Text style={[styles.label, { color: theme.text }]}>{label}</Text>
      <TextInput
        ref={ref}
        accessibilityLabel={label}
        placeholderTextColor={theme.text2}
        {...input}
        style={[styles.input, { backgroundColor: theme.surface2, color: theme.text, borderColor: error ? theme.danger : theme.border }]}
      />
      {error ? (
        <Text accessibilityLiveRegion="polite" style={[styles.error, { color: theme.dangerText }]}>
          {error}
        </Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  field: { gap: 6 },
  label: { fontSize: 14, fontWeight: '600' },
  input: { minHeight: 48, borderWidth: 1, borderRadius: radius, paddingHorizontal: 14, fontSize: 16 },
  error: { fontSize: 13 }
});
