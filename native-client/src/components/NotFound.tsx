import { StyleSheet, Text } from 'react-native';
import { router, type Href } from 'expo-router';
import { useTheme } from '../theme';
import { Button } from './Button';
import { ScreenFrame } from './ScreenFrame';

export function NotFound({ title, backHref, backLabel }: { title: string; backHref: Href; backLabel: string }) {
  const theme = useTheme();
  return (
    <ScreenFrame>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {title}
      </Text>
      <Text style={{ color: theme.text2 }}>It may have been deleted, or it belongs to another workspace.</Text>
      <Button label={`Back to ${backLabel}`} variant="secondary" onPress={() => router.dismissTo(backHref)} />
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({ title: { fontSize: 24, fontWeight: '700' } });
