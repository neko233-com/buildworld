import { useState, useCallback } from 'react';
import en from './en.json';
import zhCN from './zh-CN.json';

type Locale = 'en' | 'zh-CN';

const translations: Record<Locale, typeof en> = {
  'en': en,
  'zh-CN': zhCN,
};

function detectBrowserLocale(): Locale {
  const langs = navigator.languages || [navigator.language];
  for (const lang of langs) {
    if (lang.startsWith('zh')) return 'zh-CN';
    if (lang.startsWith('en')) return 'en';
  }
  // 无法识别时默认中文，匹配中文用户环境
  return 'zh-CN';
}

export function useI18n() {
  const [locale, setLocale] = useState<Locale>(() => {
    const saved = localStorage.getItem('locale');
    if (saved) return saved as Locale;
    return detectBrowserLocale();
  });

  const t = useCallback((key: string): string => {
    const keys = key.split('.');
    let value: any = translations[locale];
    
    for (const k of keys) {
      value = value?.[k];
    }
    
    return value || key;
  }, [locale]);

  const changeLocale = useCallback((newLocale: Locale) => {
    setLocale(newLocale);
    localStorage.setItem('locale', newLocale);
  }, []);

  return {
    locale,
    t,
    changeLocale,
    locales: Object.keys(translations) as Locale[],
  };
}
