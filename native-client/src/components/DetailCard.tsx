import type { ReactNode } from 'react';
import { Linking, Pressable, StyleSheet, Text, View } from 'react-native';
import { radius, useTheme } from '../theme';

export function DetailCard({ title, children }: { title: string; children: ReactNode }) {
  const theme = useTheme();
  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }]}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {title}
      </Text>
      {children}
    </View>
  );
}

/** One labeled value. With a link, the value opens it: mail, phone, or an in-app route. */
export function DetailRow({ label, value, url, onPress }: { label: string; value: string; url?: string; onPress?: () => void }) {
  const theme = useTheme();
  const press = onPress ?? (url ? () => void Linking.openURL(url) : undefined);
  return (
    <View style={styles.row}>
      <Text style={{ color: theme.text2, fontSize: 13 }}>{label}</Text>
      {press ? (
        <Pressable accessibilityRole="link" onPress={press} style={{ minHeight: 32, justifyContent: 'center' }}>
          <Text style={{ color: theme.accentText, fontSize: 16 }}>{value}</Text>
        </Pressable>
      ) : (
        <Text style={{ color: theme.text, fontSize: 16 }}>{value}</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { borderWidth: 1, borderRadius: radius, padding: 16, gap: 12 },
  title: { fontSize: 17, fontWeight: '600' },
  row: { gap: 2 }
});
