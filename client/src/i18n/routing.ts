import type { Locale } from './types';

export interface AlternateLink {
  hreflang: string;
  href: string;
}

export function stripLocalePrefix(pathname: string): string {
  const cleaned = pathname.replace(/^\/en(?=\/|$)/, '') || '/';
  return cleaned.startsWith('/') ? cleaned : `/${cleaned}`;
}

export function localePath(path: string, locale: Locale): string {
  if (!path) return locale === 'en' ? '/en/' : '/';
  if (/^https?:\/\//i.test(path) || path.startsWith('mailto:')) return path;

  const hashIdx = path.indexOf('#');
  const queryIdx = path.indexOf('?');
  const splitIdx =
    hashIdx === -1 ? queryIdx : queryIdx === -1 ? hashIdx : Math.min(hashIdx, queryIdx);
  const rawBase = splitIdx === -1 ? path : path.slice(0, splitIdx);
  const suffix = splitIdx === -1 ? '' : path.slice(splitIdx);

  let base = stripLocalePrefix(rawBase);
  if (base !== '/' && !base.endsWith('/') && !/\.[a-z0-9]+$/i.test(base)) {
    base = `${base}/`;
  }
  if (locale === 'en') {
    return base === '/' ? `/en/${suffix}` : `/en${base}${suffix}`;
  }
  return `${base}${suffix}`;
}

export function switchLocalePath(pathname: string, targetLocale: Locale): string {
  return localePath(stripLocalePrefix(pathname), targetLocale);
}

export function getAlternates(canonicalPath: string): AlternateLink[] {
  const base = stripLocalePrefix(canonicalPath);
  const zhHref = localePath(base, 'zh-CN');
  const enHref = localePath(base, 'en');
  return [
    { hreflang: 'zh', href: zhHref },
    { hreflang: 'en', href: enHref },
    { hreflang: 'x-default', href: enHref },
  ];
}

export function detectClientLocale(): Locale {
  if (typeof document !== 'undefined') {
    const lang = (document.documentElement.lang || '').toLowerCase();
    if (lang.startsWith('en')) return 'en';
    if (lang.startsWith('zh')) return 'zh-CN';
  }
  if (typeof window !== 'undefined') {
    const p = window.location.pathname;
    if (p === '/en' || p.startsWith('/en/')) return 'en';
  }
  return 'zh-CN';
}
