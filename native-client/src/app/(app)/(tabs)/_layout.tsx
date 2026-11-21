import { Tabs } from 'expo-router';
import { useTheme } from '../../../theme';

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
        tabBarLabelStyle: { fontSize: 15, fontWeight: '600', paddingVertical: 10 }
      }}
    >
      <Tabs.Screen name="index" options={{ title: 'Home' }} />
      <Tabs.Screen name="contacts" options={{ title: 'Contacts' }} />
      <Tabs.Screen name="companies" options={{ title: 'Companies' }} />
    </Tabs>
  );
}
