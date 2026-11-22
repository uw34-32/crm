// Not Alert.alert: that does nothing on the web export.

import { Modal, Pressable, StyleSheet, Text, View } from 'react-native';
import { radius, useTheme } from '../theme';
import { Button } from './Button';
import { ErrorNotice } from './ErrorNotice';
import { t } from '../i18n';

type Props = {
  visible: boolean;
  title: string;
  message: string;
  confirmLabel: string;
  busy?: boolean;
  error?: string;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDialog({ visible, title, message, confirmLabel, busy = false, error = '', onConfirm, onCancel }: Props) {
  const theme = useTheme();
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <Pressable accessibilityLabel={t('Close dialog')} style={styles.backdrop} onPress={busy ? undefined : onCancel}>
        <Pressable accessibilityViewIsModal style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }]} onPress={() => {}}>
          <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
            {title}
          </Text>
          <Text style={{ color: theme.text2, fontSize: 15 }}>{message}</Text>
          {error ? <ErrorNotice message={error} /> : null}
          <View style={styles.actions}>
            <Button label={t('Cancel')} variant="secondary" disabled={busy} onPress={onCancel} />
            <Button label={confirmLabel} variant="danger" busy={busy} onPress={onConfirm} />
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.5)', justifyContent: 'center', padding: 16 },
  card: { borderWidth: 1, borderRadius: radius, padding: 20, gap: 14, maxWidth: 480, width: '100%', alignSelf: 'center' },
  title: { fontSize: 20, fontWeight: '700' },
  actions: { gap: 10 }
});
