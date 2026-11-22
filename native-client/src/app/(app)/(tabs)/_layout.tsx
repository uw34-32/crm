import { Tabs } from 'expo-router';
import { useTheme } from '../../../theme';
import { t } from '../../../i18n';

export default function TabsLayout() {
  const theme = useTheme();
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: theme.accentText,
        tabBarInactiveTintColor: theme.text2,
        tabBarStyle: { backgroundColor: theme.surface, borderTopColor: theme.border },
        // No icons yet: the labels carry the navigation.
        tabBarIconStyle: { display: 'none' },
        tabBarLabelStyle: { fontSize: 14, fontWeight: '600', paddingVertical: 10 }
      }}
    >
      <Tabs.Screen name="index" options={{ title: t('Home') }} />
      <Tabs.Screen name="contacts" options={{ title: t('Contacts') }} />
      <Tabs.Screen name="companies" options={{ title: t('Companies') }} />
      <Tabs.Screen name="pipeline" options={{ title: t('Pipeline') }} />
    </Tabs>
  );
}
