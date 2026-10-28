import AsyncStorage from '@react-native-async-storage/async-storage';
import { isLang, type Lang, type MessageKey, type MessageParams, pickLanguage, translate } from '@aurarisk/shared';
import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';

const STORAGE_KEY = 'aurarisk.lang';

/** The device language, from the JS engine's default locale (e.g. "sw-KE"). */
export function deviceLanguage(): Lang {
  try {
    return pickLanguage([Intl.DateTimeFormat().resolvedOptions().locale]);
  } catch {
    return 'en';
  }
}

/** The language in use, read by the API client on each request. */
let currentLang: Lang = deviceLanguage();
export const getLang = (): Lang => currentLang;

interface I18n {
  lang: Lang;
  /** Overrides the device language and remembers the choice. */
  setLang: (lang: Lang) => void;
  t: (key: MessageKey, params?: MessageParams) => string;
}

const I18nContext = createContext<I18n | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(currentLang);

  // A saved override wins over the device language.
  useEffect(() => {
    AsyncStorage.getItem(STORAGE_KEY)
      .then((saved) => {
        if (isLang(saved)) {
          currentLang = saved;
          setLangState(saved);
        }
      })
      .catch(() => {});
  }, []);

  const setLang = useCallback((next: Lang) => {
    currentLang = next;
    setLangState(next);
    AsyncStorage.setItem(STORAGE_KEY, next).catch(() => {});
  }, []);

  const value = useMemo<I18n>(() => ({ lang, setLang, t: (key, params) => translate(lang, key, params) }), [lang, setLang]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

/** The current language, a setter, and `t()` to translate catalogue keys. */
export function useT(): I18n {
  const i18n = useContext(I18nContext);
  if (!i18n) throw new Error('useT must be used inside I18nProvider');
  return i18n;
}
