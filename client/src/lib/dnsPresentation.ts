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

const providerAliasesZH = new Map([
  ['chinanet', '中国电信'],
  ['chinanet backbone', '中国电信'],
  ['china telecom', '中国电信'],
  ['中国电信', '中国电信'],
  ['电信', '中国电信'],
  ['cmnet', '中国移动'],
  ['china mobile', '中国移动'],
  ['china mobile communications group co ltd', '中国移动'],
  ['中国移动通信', '中国移动'],
  ['中国移动', '中国移动'],
  ['移动', '中国移动'],
  ['china unicom', '中国联通'],
  ['china unicom backbone', '中国联通'],
  ['中国联通', '中国联通'],
  ['联通', '中国联通'],
  ['china broadnet', '中国广电'],
  ['中国广电', '中国广电'],
  ['广电', '中国广电'],
  ['comcast cable communications llc', 'Comcast'],
  ['comcast', 'Comcast'],
  ['xfinity', 'Comcast'],
  ['cloudflarenet', 'Cloudflare'],
  ['cloudflare inc', 'Cloudflare'],
  ['cloudflare', 'Cloudflare'],
  ['google llc', 'Google'],
  ['google public dns', 'Google'],
  ['google', 'Google'],
  ['dmit cloud services', 'DMIT'],
  ['anexia internetdienstleistungs gmbh', 'Anexia'],
]);

const providerAliasesEN = new Map([
  ['chinanet', 'China Telecom'],
  ['chinanet backbone', 'China Telecom'],
  ['china telecom', 'China Telecom'],
  ['中国电信', 'China Telecom'],
  ['电信', 'China Telecom'],
  ['cmnet', 'China Mobile'],
  ['china mobile', 'China Mobile'],
  ['china mobile communications group co ltd', 'China Mobile'],
  ['中国移动通信', 'China Mobile'],
  ['中国移动', 'China Mobile'],
  ['移动', 'China Mobile'],
  ['china unicom', 'China Unicom'],
  ['china unicom backbone', 'China Unicom'],
  ['中国联通', 'China Unicom'],
  ['联通', 'China Unicom'],
  ['china broadnet', 'China Broadnet'],
  ['中国广电', 'China Broadnet'],
  ['广电', 'China Broadnet'],
  ['comcast cable communications llc', 'Comcast'],
  ['comcast', 'Comcast'],
  ['xfinity', 'Comcast'],
  ['cloudflarenet', 'Cloudflare'],
  ['cloudflare inc', 'Cloudflare'],
  ['cloudflare', 'Cloudflare'],
  ['google llc', 'Google'],
  ['google public dns', 'Google'],
  ['google', 'Google'],
  ['dmit cloud services', 'DMIT'],
  ['anexia internetdienstleistungs gmbh', 'Anexia'],
]);

const accessProviders = new Set(['China Telecom', 'China Mobile', 'China Unicom', 'China Broadnet', 'Comcast']);

export function dnsProviderName(value: string, locale: Locale = 'zh-CN'): string {
  const name = dnsProvider(value);
  const key = name.toLowerCase().replace(/[.,]/g, '').replace(/[-\s]+/g, ' ');
  const map = locale === 'en' ? providerAliasesEN : providerAliasesZH;
  return map.get(key) ?? name;
}

export function dnsExitType(exit: DNSExit): DNSExit['type'] {
  return exit.type ?? (accessProviders.has(dnsProviderName(exit.network, 'en')) ? 'access' : undefined);
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
      const names = verdict.networks?.map((x) => dnsProviderName(x, locale)).filter(Boolean) ?? [];
      return n.crossRegion(names.length ? joinList(names, locale) : undefined);
    }
    case 'proxy-isp-bypass': {
      const names = verdict.networks?.map((x) => dnsProviderName(x, locale)).filter(Boolean) ?? [];
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

export function browserExit(info: IPIntel): DNSExit {
  let type: DNSExit['type'];
  if (info.is_vpn || info.is_proxy || info.is_tor) type = 'proxy';
  else if (info.is_mobile) type = info.is_hosting ? 'hosting' : 'access';
  else if (info.is_hosting || ['hosting', 'datacenter'].includes(info.usage_type_raw || info.usage_type)) type = 'hosting';
  else if (info.usage_type_raw === 'residential' || info.usage_type_raw === 'mobile') type = 'access';
  return {
    ip: canonicalIP(info.ip), country: (info.country ?? '').trim().toUpperCase(),
    network: dnsProvider(info.isp ?? '') || dnsProvider(info.org ?? ''),
    asn: formatASN(info.asn), type,
  };
}
