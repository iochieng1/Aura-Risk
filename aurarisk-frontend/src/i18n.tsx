import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { isLang, pickLanguage, translate, type Lang, type MessageKey, type MessageParams } from "@aurarisk/shared";

const STORAGE_KEY = "aurarisk.lang";

/** The chosen language, read by the API client on each request. */
let currentLang: Lang = "en";
export const getLang = (): Lang => currentLang;

function initialLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (isLang(saved)) return saved;
  } catch {
    // Storage blocked (private mode, sandboxed preview): fall back to the browser.
  }
  return pickLanguage(navigator.languages?.length ? navigator.languages : [navigator.language ?? "en"]);
}

interface I18n {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: MessageKey, params?: MessageParams) => string;
}

const I18nContext = createContext<I18n | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang);
  currentLang = lang;

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  const setLang = useCallback((next: Lang) => {
    currentLang = next;
    setLangState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Not remembered, but still applied for this visit.
    }
  }, []);

  const value = useMemo<I18n>(
    () => ({ lang, setLang, t: (key, params) => translate(lang, key, params) }),
    [lang, setLang]
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

/** The current language, a setter, and `t()` to translate catalogue keys. */
export function useT(): I18n {
  const i18n = useContext(I18nContext);
  if (!i18n) throw new Error("useT must be used inside I18nProvider");
  return i18n;
}
