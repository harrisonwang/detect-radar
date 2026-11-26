import { apiLang, detectClientLocale, getMessages, type Locale } from '../i18n';
import { browserExit, dnsProviderName, dnsRegion } from './dnsPresentation';
import { recentHistory, type RecentHistory } from './recentHistory';
import type { DNSExit, IPIntel } from './types';

const ENV = (import.meta as unknown as { env?: Record<string, string | boolean> }).env ?? {};

export const V4_IP_URL =
  typeof ENV.PUBLIC_V4_API_URL === 'string' && ENV.PUBLIC_V4_API_URL
    ? ENV.PUBLIC_V4_API_URL
    : 'https://4.detectradar.com/api/v1/ip';

export const V6_IP_URL =
  typeof ENV.PUBLIC_V6_API_URL === 'string' && ENV.PUBLIC_V6_API_URL
    ? ENV.PUBLIC_V6_API_URL
    : 'https://6.detectradar.com/api/v1/ip';

export type IPv6VerdictState = 'leaked' | 'clear' | 'unknown';

export type IPv6VerdictReason =
  | 'isp-bypass'
  | 'proxy-isp-bypass'
  | 'mainland-bypass'
  | 'v4-only'
  | 'v6-only'
  | 'domestic-dual'
  | 'matched-dual'
  | 'overseas-split'
  | 'both-failed';

export type IPv6Phase = 'probing' | 'comparing';

export interface StackExit extends DNSExit {
  region?: string;
  countryName?: string;
  city?: string;
  isHosting?: boolean;
  usageType?: string;
  latencyMs?: number;
}

export interface IPv6Progress {
  phase: IPv6Phase;
  v4Done: boolean;
  v6Done: boolean;
  v4: StackExit | null;
  v6: StackExit | null;
}

export interface IPv6Verdict {
  state: IPv6VerdictState;
  reason: IPv6VerdictReason;
  note: string;
  leakedCarrier?: string;
}

export interface IPv6Check {
  time: number;
  v4: StackExit | null;
  v6: StackExit | null;
  v4Error?: 'limited' | 'unreachable';
  v6Error?: 'limited' | 'unreachable';
  verdict: IPv6Verdict;
}

export type IPv6History = RecentHistory<IPv6Check>;

export interface IPv6Comparison {
  improved: boolean;
  previousVerdict?: IPv6VerdictState;
  v4Change: string;
  v6Change: string;
  summary: string | null;
}

export function getIPv6Status(locale: Locale = 'zh-CN'): Record<
  IPv6VerdictState,
  { title: string; color: string; soft: string }
> {
  const s = getMessages(locale).ipv6.status;
  return {
    leaked: { title: s.leaked, color: 'var(--leak)', soft: 'var(--leak-soft)' },
    clear: { title: s.clear, color: 'var(--sig)', soft: 'var(--sig-soft)' },
    unknown: { title: s.unknown, color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
  };
}

export const ipv6Status = getIPv6Status('zh-CN');

const HK_MO_TW_RE = /香港|澳门|澳門|台湾|臺灣|hong\s*kong|macau|macao|taiwan/i;

export function isMainlandChina(exit: StackExit | null | undefined): boolean {
  if (!exit) return false;
  if ((exit.country ?? '').trim().toUpperCase() !== 'CN') return false;
  const haystack = [exit.region, exit.countryName, exit.city].filter(Boolean).join(' ');
  return !HK_MO_TW_RE.test(haystack);
}

export function stackRegionLabel(exit: StackExit | null | undefined, locale: Locale = 'zh-CN'): string {
  if (!exit) return getMessages(locale).common.unknownRegion;
  const code = (exit.country ?? '').trim().toUpperCase();
  const haystack = [exit.region, exit.countryName, exit.city].filter(Boolean).join(' ');
  if (code === 'CN') {
    if (/香港|hong\s*kong/i.test(haystack)) return locale === 'en' ? 'Hong Kong' : '中国香港';
    if (/澳门|澳門|macau|macao/i.test(haystack)) return locale === 'en' ? 'Macao' : '中国澳门';
    if (/台湾|臺灣|taiwan/i.test(haystack)) return locale === 'en' ? 'Taiwan' : '中国台湾';
    return locale === 'en' ? 'Mainland China' : '中国大陆';
  }
  return dnsRegion(code, locale);
}

function toStackExit(info: IPIntel, latencyMs?: number): StackExit {
  const base = browserExit(info);
  const isHosting = Boolean(
    info.is_hosting ||
      info.usage_type === 'hosting' ||
      info.usage_type_raw === 'datacenter' ||
      base.type === 'hosting',
  );
  return {
    ...base,
    region: info.region || undefined,
    countryName: info.country_name || undefined,
    city: info.city || undefined,
    isHosting,
    usageType: info.usage_type || undefined,
    latencyMs,
  };
}

class StackProbeError extends Error {
  constructor(readonly kind: 'limited' | 'unreachable') {
    super(kind);
  }
}

async function probeStackEndpoint(
  baseUrl: string,
  expectedFamily: 'v4' | 'v6',
  timeoutMs: number,
  locale: Locale,
): Promise<StackExit> {
  const sep = baseUrl.includes('?') ? '&' : '?';
  const url = `${baseUrl}${sep}lang=${apiLang(locale)}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const t0 = performance.now();
  try {
    const res = await fetch(url, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    });
    if (!res.ok) {
      throw new StackProbeError(res.status === 429 ? 'limited' : 'unreachable');
    }
    const info = (await res.json()) as IPIntel;
    const ip = (info?.ip ?? '').trim();
    if (!ip) throw new StackProbeError('unreachable');
    const isV6 = ip.includes(':');
    if (expectedFamily === 'v6' ? !isV6 : isV6) {
      throw new StackProbeError('unreachable');
    }
    const latencyMs = Math.max(1, Math.round(performance.now() - t0));
    return toStackExit(info, latencyMs);
  } catch (err) {
    if (err instanceof StackProbeError) throw err;
    throw new StackProbeError('unreachable');
  } finally {
    clearTimeout(timer);
  }
}

export function stackGeoBucket(exit: StackExit | null | undefined): string {
  if (!exit) return '';
  const code = (exit.country ?? '').trim().toUpperCase();
  if (!code || code === 'LOCAL' || code === 'XX' || code === 'ZZ') return '';
  if (code === 'CN') {
    const haystack = [exit.region, exit.countryName, exit.city].filter(Boolean).join(' ');
    if (/香港|hong\s*kong/i.test(haystack)) return 'HK';
    if (/澳门|澳門|macau|macao/i.test(haystack)) return 'MO';
    if (/台湾|臺灣|taiwan/i.test(haystack)) return 'TW';
    return 'CN';
  }
  return code;
}

function isProxyOrHostingStack(exit: StackExit): boolean {
  return Boolean(
    exit.isHosting ||
      exit.type === 'hosting' ||
      exit.type === 'proxy' ||
      exit.usageType === 'hosting',
  );
}

function isAccessStack(exit: StackExit): boolean {
  if (isProxyOrHostingStack(exit)) return false;
  return Boolean(
    exit.type === 'access' ||
      exit.usageType === 'isp' ||
      isMainlandChina(exit),
  );
}

export function ipv6VerdictNote(
  verdict: Pick<IPv6Verdict, 'reason' | 'leakedCarrier'>,
  v4: StackExit | null,
  v6: StackExit | null,
  locale: Locale = 'zh-CN',
): string {
  const n = getMessages(locale).ipv6.verdictNote;
  switch (verdict.reason) {
    case 'both-failed':
      return n.bothFailed;
    case 'v4-only':
      return n.v4Only;
    case 'v6-only':
      return n.v6Only;
    case 'isp-bypass':
    case 'mainland-bypass': {
      const carrier = verdict.leakedCarrier
        ? dnsProviderName(verdict.leakedCarrier, locale)
        : v6?.network
          ? dnsProviderName(v6.network, locale)
          : '';
      return n.ispBypass(stackRegionLabel(v4, locale), stackRegionLabel(v6, locale), carrier || undefined);
    }
    case 'proxy-isp-bypass': {
      const carrier = verdict.leakedCarrier
        ? dnsProviderName(verdict.leakedCarrier, locale)
        : v6?.network
          ? dnsProviderName(v6.network, locale)
          : '';
      return n.proxyBypass(carrier || undefined);
    }
    case 'domestic-dual':
      return n.domesticDual;
    case 'matched-dual':
      return n.matchedDual(stackRegionLabel(v4, locale));
    case 'overseas-split':
      return n.overseasSplit(stackRegionLabel(v4, locale), stackRegionLabel(v6, locale));
    default:
      return '';
  }
}

export function evaluateIPv6(
  v4: StackExit | null,
  v6: StackExit | null,
  locale: Locale = 'zh-CN',
): IPv6Verdict {
  if (!v4 && !v6) {
    return {
      state: 'unknown',
      reason: 'both-failed',
      note: ipv6VerdictNote({ reason: 'both-failed' }, v4, v6, locale),
    };
  }

  if (v4 && !v6) {
    return {
      state: 'clear',
      reason: 'v4-only',
      note: ipv6VerdictNote({ reason: 'v4-only' }, v4, v6, locale),
    };
  }

  if (!v4 && v6) {
    return {
      state: 'clear',
      reason: 'v6-only',
      note: ipv6VerdictNote({ reason: 'v6-only' }, v4, v6, locale),
    };
  }

  const ipv4 = v4!;
  const ipv6 = v6!;

  const v4Geo = stackGeoBucket(ipv4);
  const v6Geo = stackGeoBucket(ipv6);
  const v4ASN = (ipv4.asn ?? '').trim().toUpperCase();
  const v6ASN = (ipv6.asn ?? '').trim().toUpperCase();
  const sameASN = Boolean(v4ASN && v6ASN && v4ASN === v6ASN);
  const v6Access = isAccessStack(ipv6);
  const v4ProxyOrHosting = isProxyOrHostingStack(ipv4);

  if (v6Access && !sameASN && v4Geo && v6Geo) {
    const carrierName = dnsProviderName(ipv6.network, 'en');
    const leakedCarrier = carrierName ? carrierName : undefined;
    if (v4Geo !== v6Geo) {
      return {
        state: 'leaked',
        reason: 'isp-bypass',
        note: ipv6VerdictNote({ reason: 'isp-bypass', leakedCarrier }, ipv4, ipv6, locale),
        ...(leakedCarrier ? { leakedCarrier } : {}),
      };
    }
    if (v4ProxyOrHosting) {
      return {
        state: 'leaked',
        reason: 'proxy-isp-bypass',
        note: ipv6VerdictNote({ reason: 'proxy-isp-bypass', leakedCarrier }, ipv4, ipv6, locale),
        ...(leakedCarrier ? { leakedCarrier } : {}),
      };
    }
  }

  if (v4Geo && v6Geo && v4Geo === v6Geo) {
    return {
      state: 'clear',
      reason: 'matched-dual',
      note: ipv6VerdictNote({ reason: 'matched-dual' }, ipv4, ipv6, locale),
    };
  }

  return {
    state: 'clear',
    reason: 'overseas-split',
    note: ipv6VerdictNote({ reason: 'overseas-split' }, ipv4, ipv6, locale),
  };
}

export async function measureIPv6(
  onProgress?: (progress: IPv6Progress) => void,
  timeouts: { v4Ms?: number; v6Ms?: number } = {},
  locale?: Locale,
): Promise<IPv6Check> {
  const effectiveLocale = locale ?? detectClientLocale();
  const v4Ms = timeouts.v4Ms ?? 5000;
  const v6Ms = timeouts.v6Ms ?? 3500;

  let v4Done = false;
  let v6Done = false;
  let v4: StackExit | null = null;
  let v6: StackExit | null = null;
  let v4Error: IPv6Check['v4Error'];
  let v6Error: IPv6Check['v6Error'];
  let phase: IPv6Phase = 'probing';

  const emit = () => {
    onProgress?.({ phase, v4Done, v6Done, v4, v6 });
  };

  emit();

  const v4Promise = probeStackEndpoint(V4_IP_URL, 'v4', v4Ms, effectiveLocale)
    .then((res) => {
      v4 = res;
    })
    .catch((err) => {
      v4Error = err instanceof StackProbeError ? err.kind : 'unreachable';
    })
    .finally(() => {
      v4Done = true;
      emit();
    });

  const v6Promise = probeStackEndpoint(V6_IP_URL, 'v6', v6Ms, effectiveLocale)
    .then((res) => {
      v6 = res;
    })
    .catch((err) => {
      v6Error = err instanceof StackProbeError ? err.kind : 'unreachable';
    })
    .finally(() => {
      v6Done = true;
      emit();
    });

  await Promise.all([v4Promise, v6Promise]);

  phase = 'comparing';
  emit();

  return {
    time: Date.now(),
    v4,
    v6,
    ...(v4Error ? { v4Error } : {}),
    ...(v6Error ? { v6Error } : {}),
    verdict: evaluateIPv6(v4, v6, effectiveLocale),
  };
}

export const ipv6History = (
  next?: IPv6Check,
  lastInPage?: IPv6Check | null,
): Promise<IPv6History> => recentHistory<IPv6Check>('detectradar-ipv6', next, lastInPage);

const STATE_RANK: Record<IPv6VerdictState, number> = {
  clear: 0,
  unknown: 1,
  leaked: 2,
};

function formatStackChange(prev: StackExit | null, cur: StackExit | null, locale: Locale): string {
  const m = getMessages(locale);
  const c = m.ipv6.compare;
  if (!prev?.ip && !cur?.ip) return c.bothUnreachable;
  if (!prev?.ip && cur?.ip) return c.nowConnected(cur.ip);
  if (prev?.ip && !cur?.ip) return c.nowDisconnected;
  if (prev!.ip.toLowerCase() === cur!.ip.toLowerCase()) return m.common.unchanged;
  return c.changed(prev!.ip, cur!.ip);
}

function stackUnchanged(prev: StackExit | null, cur: StackExit | null): boolean {
  if (!prev?.ip && !cur?.ip) return true;
  if (!prev?.ip || !cur?.ip) return false;
  return prev.ip.toLowerCase() === cur.ip.toLowerCase();
}

export function compareIPv6(
  previous: IPv6Check,
  current: IPv6Check,
  locale: Locale = 'zh-CN',
): IPv6Comparison {
  const c = getMessages(locale).ipv6.compare;
  const v4Change = formatStackChange(previous.v4, current.v4, locale);
  const v6Change = formatStackChange(previous.v6, current.v6, locale);
  const v4Unchanged = stackUnchanged(previous.v4, current.v4);
  const v6Unchanged = stackUnchanged(previous.v6, current.v6);
  const previousVerdict =
    previous.verdict.state !== current.verdict.state ? previous.verdict.state : undefined;
  const improved = STATE_RANK[current.verdict.state] < STATE_RANK[previous.verdict.state];
  const unchanged = !previousVerdict && v4Unchanged && v6Unchanged;

  return {
    improved,
    previousVerdict,
    v4Change,
    v6Change,
    summary: unchanged ? c.identical : null,
  };
}
