import { countryName } from './region';
import type { DNSExit, DNSVerdict, IPIntel } from './types';

export const DNS_POLICY_VERSION = 2;

export const dnsStatus = {
  suspected: { title: '泄露', color: 'var(--leak)', soft: 'var(--leak-soft)' },
  clear: { title: '未泄露', color: 'var(--sig)', soft: 'var(--sig-soft)' },
  unknown: { title: '无法判断', color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
  unobserved: { title: '无 DNS 结果', color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
};

export function dnsRegion(code: string): string {
  if (code === 'LOCAL') return '本地网络';
  return /^[A-Z]{2}$/.test(code) && code !== 'XX' && code !== 'ZZ' ? countryName(code) : '地区未知';
}

// A provider may be blank or returned as a numeric host name. Do not use
// addresses as display labels in summaries or comparisons.
export function dnsProvider(name: string): string {
  const label = name.trim();
  return /^(unknown|n\/a|-)$/i.test(label) ||
    /(?:\d{1,3}[.-]){3}\d{1,3}|[0-9a-f]*:[0-9a-f]*:/i.test(label) ? '' : label;
}

// Presentation aliases are also available while the exit lookup is on screen
// and the DNS assessment has yet to return its normalized observations.
const providerAliases = new Map([
  ['chinanet', '中国电信'],
  ['chinanet backbone', '中国电信'],
  ['china telecom', '中国电信'],
  ['电信', '中国电信'],
  ['cmnet', '中国移动'],
  ['china mobile', '中国移动'],
  ['china mobile communications group co ltd', '中国移动'],
  ['中国移动通信', '中国移动'],
  ['移动', '中国移动'],
  ['china unicom', '中国联通'],
  ['china unicom backbone', '中国联通'],
  ['联通', '中国联通'],
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
const accessProviders = new Set(['中国电信', '中国移动', '中国联通', 'Comcast']);

export function dnsProviderName(value: string): string {
  const name = dnsProvider(value);
  const key = name.toLowerCase().replace(/[.,]/g, '').replace(/[-\s]+/g, ' ');
  return providerAliases.get(key) ?? name;
}

export function dnsExitType(exit: DNSExit): DNSExit['type'] {
  return exit.type ?? (accessProviders.has(dnsProviderName(exit.network)) ? 'access' : undefined);
}

export function exitTypeLabel(type: DNSExit['type']): string {
  switch (type) {
    case 'proxy': return '代理特征';
    case 'hosting': return '机房 IP';
    case 'access': return '普通网络';
    default: return '';
  }
}

export function dnsVerdictNote(verdict: DNSVerdict | undefined): string {
  if (!verdict) return '';
  switch (verdict.reason) {
    case 'mainland-carrier':
      return `出口 IP 在海外，测到了${verdict.networks?.join('、') || '国内运营商'}的 DNS。`;
    case 'domestic-exit':
      return '出口 IP 在中国大陆，本次 DNS 归属未见异常。';
    case 'no-mainland-carrier':
      return '本次未测到国内运营商 DNS。';
    case 'missing-exit':
      return '未获取出口 IP 或归属地。';
    case 'partial-samples':
      return '部分测试未收到 DNS 结果。';
    case 'unclassified-resolver':
      return '部分 DNS 的网络归属不明确。';
    case 'assessment-unavailable':
      return '判定服务暂时连不上。';
    default:
      return '';
  }
}

export function browserExit(info: IPIntel): DNSExit {
  let type: DNSExit['type'];
  if (info.is_vpn || info.is_proxy || info.is_tor) type = 'proxy';
  else if (info.is_mobile) type = info.is_hosting ? 'hosting' : 'access';
  else if (info.is_hosting || ['hosting', 'datacenter'].includes(info.usage_type_raw || info.usage_type)) type = 'hosting';
  else if (info.usage_type_raw === 'residential' || info.usage_type_raw === 'mobile') type = 'access';
  return {
    ip: info.ip, country: (info.country ?? '').trim().toUpperCase(),
    network: dnsProvider(info.isp ?? '') || dnsProvider(info.org ?? ''),
    asn: info.asn || undefined, type,
  };
}
