// A screen's frame: the safe area, a scrolling body, and a header with a way back.

import type { ReactNode } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { router, type Href } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTheme } from '../theme';
import { t } from '../i18n';

type Props = { backLabel?: string; backHref?: Href; actions?: ReactNode; children: ReactNode };

export function ScreenFrame({ backLabel, backHref, actions, children }: Props) {
  const theme = useTheme();
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }} edges={['top', 'left', 'right']}>
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        {backLabel ? (
          <View style={styles.header}>
            <Pressable
              accessibilityRole="link"
              accessibilityLabel={t('Back to {place}', { place: backLabel })}
              hitSlop={8}
              onPress={() => (router.canGoBack() ? router.back() : router.replace(backHref ?? '/'))}
            >
              <Text style={{ color: theme.accentText, fontSize: 16 }}>‹ {backLabel}</Text>
            </Pressable>
            <View style={styles.actions}>{actions}</View>
          </View>
        ) : null}
        {children}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: 16, gap: 16, maxWidth: 640, width: '100%', alignSelf: 'center' },
  header: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', minHeight: 44 },
  actions: { flexDirection: 'row', gap: 16 }
});
