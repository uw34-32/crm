import type { ReactNode } from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { radius, useTheme } from '../theme';
import { LanguageSwitcher } from './LanguageSwitcher';

export function AuthScreen({ title, children, footer }: { title: string; children: ReactNode; footer: ReactNode }) {
  const theme = useTheme();
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
          <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }]}>
            <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
              {title}
            </Text>
            <View style={styles.body}>{children}</View>
          </View>
          <View style={styles.footer}>{footer}</View>
          <LanguageSwitcher />
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  scroll: { flexGrow: 1, justifyContent: 'center', padding: 16, gap: 16, maxWidth: 480, width: '100%', alignSelf: 'center' },
  card: { borderWidth: 1, borderRadius: radius, padding: 20, gap: 16 },
  title: { fontSize: 24, fontWeight: '700' },
  body: { gap: 16 },
  footer: { alignItems: 'center' }
});
