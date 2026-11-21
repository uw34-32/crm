import { StyleSheet, TextInput } from 'react-native';
import { radius, useTheme } from '../theme';

export function SearchField({ value, onChangeText, placeholder }: { value: string; onChangeText: (text: string) => void; placeholder: string }) {
  const theme = useTheme();
  return (
    <TextInput
      accessibilityLabel={placeholder}
      placeholder={placeholder}
      placeholderTextColor={theme.text2}
      value={value}
      onChangeText={onChangeText}
      returnKeyType="search"
      autoCapitalize="none"
      autoCorrect={false}
      clearButtonMode="while-editing"
      style={[styles.input, { backgroundColor: theme.surface2, color: theme.text, borderColor: theme.border }]}
    />
  );
}

const styles = StyleSheet.create({ input: { minHeight: 44, borderWidth: 1, borderRadius: radius, paddingHorizontal: 14, fontSize: 16 } });
