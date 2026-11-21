import { StyleSheet, Text, View } from 'react-native';
import { initials } from '../lib/records/format';
import { useTheme } from '../theme';

export function RecordAvatar({ name, size = 44 }: { name: string; size?: number }) {
  const theme = useTheme();
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={[styles.avatar, { width: size, height: size, borderRadius: size / 2, backgroundColor: theme.surface2, borderColor: theme.border }]}
    >
      <Text style={{ color: theme.accentText, fontWeight: '700', fontSize: size * 0.36 }}>{initials(name)}</Text>
    </View>
  );
}

const styles = StyleSheet.create({ avatar: { alignItems: 'center', justifyContent: 'center', borderWidth: 1 } });
