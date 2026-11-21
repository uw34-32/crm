import { StyleSheet, Text, View } from 'react-native';
import { radius, useTheme } from '../theme';
import { Button } from './Button';

export function ErrorNotice({ message, onRetry }: { message: string; onRetry?: () => void }) {
  const theme = useTheme();
  return (
    <View accessibilityRole="alert" style={[styles.box, { borderColor: theme.danger, backgroundColor: theme.surface }]}>
      <Text style={[styles.text, { color: theme.dangerText }]}>{message}</Text>
      {onRetry ? <Button label="Try again" variant="secondary" onPress={onRetry} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  box: { borderWidth: 1, borderRadius: radius, padding: 14, gap: 12 },
  text: { fontSize: 15 }
});
