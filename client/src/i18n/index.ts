import type { Locale } from './types';
import { zh, type Messages } from './messages/zh';
import { en } from './messages/en';

export type { Locale } from './types';
export type { Messages } from './messages/zh';
export { DEFAULT_LOCALE, isEnglish, apiLang } from './types';
export { localePath, stripLocalePrefix, switchLocalePath, getAlternates, detectClientLocale } from './routing';
export { countryName, languageName, formatTimezoneZone, joinList } from './format';

export function getMessages(locale: Locale = 'zh-CN'): Messages {
  return locale === 'en' ? en : zh;
}
