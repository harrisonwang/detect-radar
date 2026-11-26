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

export const ipv6Status: Record<
  IPv6VerdictState,
  { title: string; color: string; soft: string }
> = {
  leaked: { title: '泄露', color: 'var(--leak)', soft: 'var(--leak-soft)' },
  clear: { title: '未泄露', color: 'var(--sig)', soft: 'var(--sig-soft)' },
  unknown: { title: '无法判断', color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
};

const HK_MO_TW_RE = /香港|澳门|澳門|台湾|臺灣|hong\s*kong|macau|macao|taiwan/i;

export function isMainlandChina(exit: StackExit | null | undefined): boolean {
  if (!exit) return false;
  if ((exit.country ?? '').trim().toUpperCase() !== 'CN') return false;
  const haystack = [exit.region, exit.countryName, exit.city].filter(Boolean).join(' ');
  return !HK_MO_TW_RE.test(haystack);
}

export function stackRegionLabel(exit: StackExit | null | undefined): string {
  if (!exit) return '地区未知';
  const code = (exit.country ?? '').trim().toUpperCase();
  const haystack = [exit.region, exit.countryName, exit.city].filter(Boolean).join(' ');
  if (code === 'CN') {
    if (/香港|hong\s*kong/i.test(haystack)) return '中国香港';
    if (/澳门|澳門|macau|macao/i.test(haystack)) return '中国澳门';
    if (/台湾|臺灣|taiwan/i.test(haystack)) return '中国台湾';
    return '中国大陆';
  }
  return dnsRegion(code);
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
  url: string,
  expectedFamily: 'v4' | 'v6',
  timeoutMs: number,
): Promise<StackExit> {
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

export function evaluateIPv6(v4: StackExit | null, v6: StackExit | null): IPv6Verdict {
  if (!v4 && !v6) {
    return {
      state: 'unknown',
      reason: 'both-failed',
      note: '未能连通 IPv4 与 IPv6 探测服务。',
    };
  }

  if (v4 && !v6) {
    return {
      state: 'clear',
      reason: 'v4-only',
      note: '当前网络仅连通 IPv4，不存在 IPv6 泄露。',
    };
  }

  if (!v4 && v6) {
    return {
      state: 'clear',
      reason: 'v6-only',
      note: '当前仅连通 IPv6 出口，未测得 IPv4 地址。',
    };
  }

  const ipv4 = v4!;
  const ipv6 = v6!;

  const v4Mainland = isMainlandChina(ipv4);
  const v6Mainland = isMainlandChina(ipv6);
  const v6IsHosting = Boolean(ipv6.isHosting || ipv6.type === 'hosting');

  if (!v4Mainland && v6Mainland && !v6IsHosting) {
    const carrierName = dnsProviderName(ipv6.network);
    const leakedCarrier = carrierName && carrierName !== '网络未知' ? carrierName : undefined;
    return {
      state: 'leaked',
      reason: 'mainland-bypass',
      note: `IPv4 出口在海外（${stackRegionLabel(ipv4)}），但 IPv6 经${leakedCarrier || '国内运营商'}直连，存在 IPv6 泄露。`,
      ...(leakedCarrier ? { leakedCarrier } : {}),
    };
  }

  if (v4Mainland && v6Mainland) {
    return {
      state: 'clear',
      reason: 'domestic-dual',
      note: 'IPv4 与 IPv6 均在中国大陆直连，未见异常。',
    };
  }

  const r4 = stackRegionLabel(ipv4);
  const r6 = stackRegionLabel(ipv6);
  if (r4 === r6) {
    return {
      state: 'clear',
      reason: 'matched-dual',
      note: `IPv4 与 IPv6 出口均位于${r4}，未见泄露。`,
    };
  }

  return {
    state: 'clear',
    reason: 'overseas-split',
    note: `未测到国内运营商 IPv6 直连（IPv4 位于${r4}，IPv6 位于${r6}，常见于分流节点或 WARP）。`,
  };
}

export async function measureIPv6(
  onProgress?: (progress: IPv6Progress) => void,
  timeouts: { v4Ms?: number; v6Ms?: number } = {},
): Promise<IPv6Check> {
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

  const v4Promise = probeStackEndpoint(V4_IP_URL, 'v4', v4Ms)
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

  const v6Promise = probeStackEndpoint(V6_IP_URL, 'v6', v6Ms)
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
    verdict: evaluateIPv6(v4, v6),
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

function formatStackChange(prev: StackExit | null, cur: StackExit | null): string {
  if (!prev?.ip && !cur?.ip) return '两次均未连通';
  if (!prev?.ip && cur?.ip) return `本次连通 ${cur.ip}`;
  if (prev?.ip && !cur?.ip) return '本次未连通';
  if (prev!.ip.toLowerCase() === cur!.ip.toLowerCase()) return '未变';
  return `从 ${prev!.ip} 变为 ${cur!.ip}`;
}

export function compareIPv6(previous: IPv6Check, current: IPv6Check): IPv6Comparison {
  const v4Change = formatStackChange(previous.v4, current.v4);
  const v6Change = formatStackChange(previous.v6, current.v6);
  const previousVerdict =
    previous.verdict.state !== current.verdict.state ? previous.verdict.state : undefined;
  const improved = STATE_RANK[current.verdict.state] < STATE_RANK[previous.verdict.state];
  const unchanged = !previousVerdict && v4Change === '未变' && v6Change === '未变';

  return {
    improved,
    previousVerdict,
    v4Change,
    v6Change,
    summary: unchanged ? 'IPv4 与 IPv6 出口均与上次一致' : null,
  };
}
