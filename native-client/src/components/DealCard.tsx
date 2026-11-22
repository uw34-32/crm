import { Pressable, StyleSheet, Text, View } from 'react-native';
import { router } from 'expo-router';
import type { Deal } from '../lib/deals/board';
import { formatMoney } from '../lib/deals/money';
import { radius, useTheme } from '../theme';
import { t } from '../i18n';

type Props = { deal: Deal; busy: boolean; onMove: () => void };

export function DealCard({ deal, busy, onMove }: Props) {
  const theme = useTheme();
  const party = deal.company_name ?? deal.contact_name ?? '';
  const value = deal.value_minor > 0 ? formatMoney(deal.value_minor, deal.currency) : '';
  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border, opacity: busy ? 0.6 : 1 }]}>
      <Pressable accessibilityRole="link" accessibilityLabel={deal.title} onPress={() => router.push(`/deals/${deal.id}`)} style={{ flex: 1, gap: 2 }}>
        <Text numberOfLines={2} style={[styles.title, { color: theme.text }]}>
          {deal.title}
        </Text>
        {party ? (
          <Text numberOfLines={1} style={{ color: theme.text2 }}>
            {party}
          </Text>
        ) : null}
        {value ? <Text style={{ color: theme.text, fontWeight: '600' }}>{value}</Text> : null}
        {deal.expected_close_date ? <Text style={{ color: theme.text2, fontSize: 13 }}>Closes {deal.expected_close_date}</Text> : null}
      </Pressable>
      <Pressable accessibilityRole="button" accessibilityLabel={t('Move {name}', { name: deal.title })} disabled={busy} hitSlop={8} onPress={onMove} style={styles.move}>
        <Text style={{ color: theme.accentText, fontWeight: '600' }}>{t('Move')}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: 'row', alignItems: 'center', gap: 12, borderWidth: 1, borderRadius: radius, padding: 14, minHeight: 72 },
  title: { fontSize: 16, fontWeight: '600' },
  move: { minHeight: 44, minWidth: 44, alignItems: 'center', justifyContent: 'center' }
});
