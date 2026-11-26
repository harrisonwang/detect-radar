import { countryName, getMessages, joinList, type Locale } from '../i18n';
import { canonicalIP } from './ip';
import type { DNSExit, DNSVerdict, IPIntel } from './types';

export const DNS_POLICY_VERSION = 3;

export function getDnsStatus(locale: Locale = 'zh-CN') {
  const s = getMessages(locale).dns.status;
  return {
    suspected: { title: s.suspected, color: 'var(--leak)', soft: 'var(--leak-soft)' },
    clear: { title: s.clear, color: 'var(--sig)', soft: 'var(--sig-soft)' },
    unknown: { title: s.unknown, color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
    unobserved: { title: s.unobserved, color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
  };
}

export const dnsStatus = getDnsStatus('zh-CN');

export function dnsRegion(code: string, locale: Locale = 'zh-CN'): string {
  const m = getMessages(locale);
  if (code === 'LOCAL') return m.common.localNetwork;
  return /^[A-Z]{2}$/.test(code) && code !== 'XX' && code !== 'ZZ'
    ? countryName(code, locale)
    : m.common.unknownRegion;
}

export function dnsProvider(name: string): string {
  const label = name.trim();
  return /^(unknown|n\/a|-)$/i.test(label) ||
    /(?:\d{1,3}[.-]){3}\d{1,3}|[0-9a-f]*:[0-9a-f]*:/i.test(label) ? '' : label;
}

export function dnsExitType(exit: DNSExit): DNSExit['type'] {
  return exit.type;
}

export function exitTypeLabel(type: DNSExit['type'], locale: Locale = 'zh-CN'): string {
  const labels = getMessages(locale).common.exitType;
  switch (type) {
    case 'proxy': return labels.proxy;
    case 'hosting': return labels.hosting;
    case 'access': return labels.access;
    default: return '';
  }
}

export function dnsVerdictNote(
  verdict: DNSVerdict | undefined,
  tense: 'current' | 'previous' = 'current',
  locale: Locale = 'zh-CN',
): string {
  if (!verdict) return '';
  const cur = tense === 'current';
  const n = getMessages(locale).dns.verdictNote;
  switch (verdict.reason) {
    case 'cross-region-isp':
    case 'mainland-carrier': {
      const names = verdict.networks?.map((x) => dnsProvider(x)).filter(Boolean) ?? [];
      return n.crossRegion(names.length ? joinList(names, locale) : undefined);
    }
    case 'proxy-isp-bypass': {
      const names = verdict.networks?.map((x) => dnsProvider(x)).filter(Boolean) ?? [];
      return n.proxyBypass(names.length ? joinList(names, locale) : undefined);
    }
    case 'same-isp-exit':
    case 'domestic-exit':
      return cur ? n.sameIspCurrent : n.sameIspPrevious;
    case 'public-or-hosting-only':
      return cur ? n.publicHostingCurrent : n.publicHostingPrevious;
    case 'no-leaked-isp':
    case 'no-mainland-carrier':
      return cur ? n.noLeakedCurrent : n.noLeakedPrevious;
    case 'missing-exit':
      return n.missingExit;
    case 'partial-samples':
      return n.partialSamples;
    case 'unclassified-resolver':
      return n.unclassified;
    case 'assessment-unavailable':
      return n.unavailable;
    default:
      return '';
  }
}

export function isLiveResolverSuspect(
  exit: DNSExit | null | undefined,
  resolver: { country?: string; asn?: string; network_type?: string },
): boolean {
  if (!exit?.country || resolver.network_type !== 'access' || !resolver.country) return false;
  const exitASN = (exit.asn ?? '').trim().toUpperCase();
  const rASN = (resolver.asn ?? '').trim().toUpperCase();
  if (exitASN && rASN && exitASN === rASN) return false;
  if (resolver.country.toUpperCase() !== exit.country.toUpperCase()) return true;
  return exit.type === 'proxy' || exit.type === 'hosting';
}

function formatASN(raw: string | undefined): string | undefined {
  const s = (raw ?? '').trim().toUpperCase();
  if (!s) return undefined;
  const num = s.startsWith('AS') ? s.slice(2) : s;
  return /^\d+$/.test(num) ? `AS${num}` : s;
}

export function normalizeGeoBucket(info: Pick<IPIntel, 'country' | 'region' | 'country_name' | 'city'>): string {
  const code = (info.country ?? '').trim().toUpperCase();
  if (code === 'CN') {
    const haystack = [info.region, info.country_name, info.city].filter(Boolean).join(' ');
    if (/香港|hong\s*kong/i.test(haystack)) return 'HK';
    if (/澳门|澳門|macau|macao/i.test(haystack)) return 'MO';
    if (/台湾|臺灣|taiwan/i.test(haystack)) return 'TW';
  }
  return code;
}

export function browserExit(info: IPIntel): DNSExit {
  let type: DNSExit['type'];
  if (info.is_vpn || info.is_proxy || info.is_tor) type = 'proxy';
  else if (info.is_mobile) type = info.is_hosting ? 'hosting' : 'access';
  else if (info.is_hosting || ['hosting', 'datacenter'].includes(info.usage_type_raw || info.usage_type)) type = 'hosting';
  else if (info.usage_type === 'isp' || ['residential', 'mobile', 'isp'].includes(info.usage_type_raw ?? '')) type = 'access';
  return {
    ip: canonicalIP(info.ip), country: normalizeGeoBucket(info),
    network: dnsProvider(info.isp ?? '') || dnsProvider(info.org ?? ''),
    asn: formatASN(info.asn), type,
  };
}
