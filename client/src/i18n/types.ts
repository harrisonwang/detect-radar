export type Locale = 'zh-CN' | 'en';

export const DEFAULT_LOCALE: Locale = 'zh-CN';

export function isEnglish(locale: Locale): boolean {
  return locale === 'en';
}

export function apiLang(locale: Locale): 'zh' | 'en' {
  return locale === 'en' ? 'en' : 'zh';
}
