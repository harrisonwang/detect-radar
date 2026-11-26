import type { Locale } from './types';

const CN_NUM = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九', '十', '十一', '十二', '十三'];

export function countryName(code: string, locale: Locale = 'zh-CN'): string {
  const upper = (code || '').trim().toUpperCase();
  if (!upper) return locale === 'en' ? 'Unknown' : '地区未知';
  if (locale === 'zh-CN') {
    if (upper === 'HK') return '中国香港';
    if (upper === 'MO') return '中国澳门';
    if (upper === 'TW') return '中国台湾';
  }
  try {
    return new Intl.DisplayNames([locale], { type: 'region' }).of(upper) || upper;
  } catch {
    return upper;
  }
}

export function languageName(code: string, locale: Locale = 'zh-CN'): string {
  const raw = (code || '').trim();
  if (!raw) return locale === 'en' ? 'Unknown' : '未知';
  const lower = raw.toLowerCase();
  if (locale === 'zh-CN') {
    if (lower === 'zh' || lower === 'zh-cn' || lower === 'zh-sg' || lower === 'zh-hans') return '简体中文';
    if (lower === 'zh-tw' || lower === 'zh-hk' || lower === 'zh-mo' || lower === 'zh-hant') return '繁体中文';
  }
  try {
    const dn = new Intl.DisplayNames([locale], { type: 'language' });
    return dn.of(raw) || dn.of(lower.split('-')[0]) || raw;
  } catch {
    return raw;
  }
}

function offsetToZone(mins: number, locale: Locale): string {
  if (mins === 0) return locale === 'en' ? 'UTC' : '零时区';
  const abs = Math.abs(mins);
  const h = Math.floor(abs / 60);
  const m = abs % 60;
  if (locale === 'en') {
    const sign = mins > 0 ? '+' : '-';
    return m === 0 ? `UTC${sign}${h}` : `UTC${sign}${h}:${String(m).padStart(2, '0')}`;
  }
  const hz = CN_NUM[h] ?? String(h);
  const dir = mins > 0 ? '东' : '西';
  return m === 0 ? `${dir}${hz}区` : `${dir}${hz}区${m}分`;
}

export function formatTimezoneZone(tz: string, locale: Locale = 'zh-CN'): string {
  if (!tz || tz === 'unknown' || tz === '-') return locale === 'en' ? 'Unknown' : '未知';
  if (tz === 'UTC' || tz === 'GMT' || tz === 'Z') return locale === 'en' ? 'UTC' : '零时区';
  const m = tz.match(/^([+-])(\d{2}):?(\d{2})$/);
  if (m) {
    const mins = (parseInt(m[2], 10) * 60 + parseInt(m[3], 10)) * (m[1] === '-' ? -1 : 1);
    return offsetToZone(mins, locale);
  }
  try {
    const name =
      new Intl.DateTimeFormat('en-US', {
        timeZone: tz,
        timeZoneName: 'longOffset',
      })
        .formatToParts(new Date())
        .find((p) => p.type === 'timeZoneName')?.value || '';
    const mm = name.match(/GMT([+-])(\d{1,2})(?::(\d{2}))?/);
    if (mm) {
      const mins =
        (parseInt(mm[2], 10) * 60 + parseInt(mm[3] || '0', 10)) * (mm[1] === '-' ? -1 : 1);
      return offsetToZone(mins, locale);
    }
    if (name === 'GMT') return offsetToZone(0, locale);
  } catch {
  }
  return tz;
}

export function joinList(items: string[], locale: Locale = 'zh-CN'): string {
  return items.filter(Boolean).join(locale === 'en' ? ', ' : '、');
}
