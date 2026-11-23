import type { MessageKey } from '@aurarisk/shared';
import { Tabs } from 'expo-router';
import { Text } from 'react-native';
import { useT } from '../../i18n';

const TABS: { name: string; title: MessageKey; icon: string }[] = [
  { name: 'index', title: 'mobile.tabRisk', icon: '⚠️' },
  { name: 'map', title: 'mobile.tabMap', icon: '🗺️' },
  { name: 'report', title: 'mobile.tabReport', icon: '📝' },
  { name: 'alerts', title: 'mobile.tabAlerts', icon: '🔔' },
  { name: 'settings', title: 'mobile.tabSettings', icon: '⚙️' },
];

export default function TabsLayout() {
  const { t } = useT();
  return (
    <Tabs screenOptions={{ tabBarActiveTintColor: '#0b5cad' }}>
      {TABS.map((tab) => (
        <Tabs.Screen
          key={tab.name}
          name={tab.name}
          options={{
            title: t(tab.title),
            // Decorative: the label already names the tab for screen readers.
            tabBarIcon: ({ size }) => (
              <Text style={{ fontSize: size * 0.8 }} accessible={false} importantForAccessibility="no">
                {tab.icon}
              </Text>
            ),
          }}
        />
      ))}
    </Tabs>
  );
}
