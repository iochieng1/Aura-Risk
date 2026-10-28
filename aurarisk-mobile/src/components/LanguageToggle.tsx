import { LANGUAGES } from '@aurarisk/shared';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useT } from '../i18n';

/** English / Kiswahili switch. The app follows the device language until one is picked here. */
export default function LanguageToggle() {
  const { lang, setLang, t } = useT();
  return (
    <View style={styles.row} accessibilityRole="radiogroup" accessibilityLabel={t('ui.language')}>
      {LANGUAGES.map((l) => (
        <Pressable
          key={l.value}
          onPress={() => setLang(l.value)}
          style={[styles.chip, lang === l.value && styles.chipSelected]}
          accessibilityRole="radio"
          accessibilityState={{ selected: lang === l.value }}
          accessibilityLanguage={l.value}
        >
          <Text style={lang === l.value ? styles.chipTextSelected : undefined}>{l.label}</Text>
        </Pressable>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', justifyContent: 'flex-end', gap: 8 },
  chip: { paddingVertical: 4, paddingHorizontal: 12, borderRadius: 16, borderWidth: 1, borderColor: '#ccc' },
  chipSelected: { backgroundColor: '#0b5cad', borderColor: '#0b5cad' },
  chipTextSelected: { color: '#fff' },
});
