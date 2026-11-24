import { ApiError, assessWebRTCCandidates, getBrowserExit } from './api';
import {
  gatherRTCCandidates,
  type RTCCandidateDetail,
  type RTCGatherResult,
  type RTCLocalScope,
} from './collectors/leaks/webrtc';
import { browserExit, dnsProviderName, dnsRegion } from './dnsPresentation';
import {
  canonicalIP,
  isIPv6Address,
  sameExitOrPrefix64,
  sameIP,
  sameIPv6Prefix64,
} from './ip';
import { recentHistory, type RecentHistory } from './recentHistory';
import type { DNSExit, WebRTCAssessmentResponse, WebRTCNetworkInfo } from './types';

export type WebRTCVerdictState = 'leaked' | 'clear' | 'unknown';

export type WebRTCPhase = 'gathering' | 'collected' | 'comparing';

export interface WebRTCProgress {
  phase: WebRTCPhase;
  exit: DNSExit | null;
  candidateCount: number;
  publicIPs: string[];
  localIPs: string[];
  mdnsHosts: string[];
  ipv6Global: string | null;
  apiSupported: boolean;
}

export type WebRTCVerdictReason =
  | 'public-leak'
  | 'proxy-bypass'
  | 'public-mismatch'
  | 'local-exposed'
  | 'matches-exit'
  | 'same-prefix'
  | 'dual-stack'
  | 'same-network'
  | 'mdns-protected'
  | 'no-candidates'
  | 'disabled'
  | 'missing-exit'
  | 'assessment-unavailable';

export interface WebRTCVerdict {
  state: WebRTCVerdictState;
  reason: WebRTCVerdictReason;
  note: string;
  suspectIPs: string[];
}

export interface WebRTCCheck {
  time: number;
  exit: DNSExit | null;
  apiSupported: boolean;
  publicIPs: string[];
  publicCandidates?: WebRTCNetworkInfo[];
  stunIPs: string[];
  localIPs: string[];
  mdnsHosts: string[];
  ipv6Global: string | null;
  candidates: RTCCandidateDetail[];
  verdict: WebRTCVerdict;
  exitError?: 'limited' | 'unavailable';
}

export type WebRTCHistory = RecentHistory<WebRTCCheck>;

export interface WebRTCComparison {
  improved: boolean;
  previousVerdict?: WebRTCVerdictState;
  addedPublicIPs: Set<string>;
  removedPublicIPs: string[];
  addedLocalIPs: Set<string>;
  removedLocalIPs: string[];
  publicChange: string;
  localChange: string;
  exitChange: string;
  summary: string | null;
}

export const webrtcStatus: Record<
  WebRTCVerdictState,
  { title: string; color: string; soft: string }
> = {
  leaked: { title: '泄露', color: 'var(--leak)', soft: 'var(--leak-soft)' },
  clear: { title: '未泄露', color: 'var(--sig)', soft: 'var(--sig-soft)' },
  unknown: { title: '无法判断', color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
};

const LOCAL_SCOPE_LABELS: Record<RTCLocalScope, string> = {
  lan: '局域网',
  cgnat: 'CGNAT / Tailscale',
  fakeip: '虚拟网卡 / Fake-IP',
  'link-local': '链路本地',
  ula: 'IPv6 ULA',
  loopback: '本地回环',
};

export function localScopeLabel(scope?: RTCLocalScope): string {
  return scope ? LOCAL_SCOPE_LABELS[scope] : '内网';
}

function isExpectedDualStack(exitIP: string, extraPublicIPs: string[]): boolean {
  if (!exitIP || extraPublicIPs.length === 0) return false;
  if (!isIPv6Address(exitIP) && extraPublicIPs.every((ip) => isIPv6Address(ip))) return true;
  if (isIPv6Address(exitIP) && extraPublicIPs.every((ip) => !isIPv6Address(ip))) return true;
  return false;
}

function dedupeCanonicalIPs(ips: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of ips) {
    const canon = canonicalIP(raw);
    if (!canon || seen.has(canon)) continue;
    seen.add(canon);
    out.push(canon);
  }
  return out;
}

export function evaluateWebRTC(exit: DNSExit | null, rtc: RTCGatherResult): WebRTCVerdict {
  if (!rtc.apiSupported) {
    return {
      state: 'clear',
      reason: 'disabled',
      note: '浏览器已禁用 WebRTC，未暴露任何地址。',
      suspectIPs: [],
    };
  }

  const publicIPs = dedupeCanonicalIPs(rtc.publicIPs);
  const exitIP = canonicalIP(exit?.ip);
  if (publicIPs.length > 0 && !exitIP) {
    return {
      state: 'unknown',
      reason: 'missing-exit',
      note: '已测到 WebRTC 公网 IP，但未获取到出口 IP 进行比对。',
      suspectIPs: [],
    };
  }

  const extraPublicIPs = exitIP
    ? publicIPs.filter((ip) => !sameExitOrPrefix64(ip, exitIP))
    : publicIPs;

  if (extraPublicIPs.length > 0) {
    if (isExpectedDualStack(exitIP, extraPublicIPs)) {
      if (exit?.type === 'proxy' || exit?.type === 'hosting') {
        return {
          state: 'unknown',
          reason: 'assessment-unavailable',
          note: '测到另一协议栈的公网 IP，但暂未完成归属核对，无法确认是否同属当前代理网络。',
          suspectIPs: [],
        };
      }
      return {
        state: 'clear',
        reason: 'dual-stack',
        note: '测到另一协议栈的公网 IP，属正常 IPv4/IPv6 双栈。',
        suspectIPs: [],
      };
    }
    return {
      state: 'leaked',
      reason: 'public-leak',
      note: 'WebRTC 测到的公网 IP 与当前出口不一致。',
      suspectIPs: extraPublicIPs,
    };
  }

  if (publicIPs.length > 0 && exitIP && publicIPs.some((ip) => !sameIP(ip, exitIP) && sameIPv6Prefix64(ip, exitIP))) {
    return {
      state: 'clear',
      reason: 'same-prefix',
      note: 'WebRTC 测到的 IPv6 地址与当前出口属于同一 /64 前缀（临时隐私地址），未发生泄露。',
      suspectIPs: [],
    };
  }

  if (publicIPs.length === 0 && rtc.localIPs.length > 0) {
    return {
      state: 'clear',
      reason: 'local-exposed',
      note: '未测到额外公网 IP（仅读取到本机内网地址）。',
      suspectIPs: [],
    };
  }

  if (publicIPs.length > 0) {
    return {
      state: 'clear',
      reason: 'matches-exit',
      note: 'WebRTC 测到的公网 IP 与当前出口一致。',
      suspectIPs: [],
    };
  }

  if (rtc.mdnsHosts.length > 0) {
    return {
      state: 'clear',
      reason: 'mdns-protected',
      note: '仅返回 mDNS（*.local），未暴露公网或内网 IP。',
      suspectIPs: [],
    };
  }

  return {
    state: 'clear',
    reason: 'no-candidates',
    note: '本次未测到 WebRTC 地址。',
    suspectIPs: [],
  };
}

function formatCandidateLabel(info: WebRTCNetworkInfo | undefined): string {
  if (!info) return '';
  const region = info.country ? dnsRegion(info.country) : '';
  const validRegion = region && region !== '地区未知' ? region : '';
  const provider = dnsProviderName(info.network ?? '');
  if (validRegion && provider) {
    return provider.startsWith(validRegion) ? provider : `${validRegion} ${provider}`;
  }
  return provider || validRegion;
}

export function verdictFromAssessment(
  res: WebRTCAssessmentResponse,
  exit: DNSExit | null,
): WebRTCVerdict {
  const suspectIPs = dedupeCanonicalIPs(res.verdict.suspect_ips ?? []);
  const reason = (res.verdict.reason || 'matches-exit') as WebRTCVerdictReason;

  if (res.verdict.state === 'leaked') {
    const firstSuspect = res.candidates.find((c) => c.relation === 'leak');
    const label = formatCandidateLabel(firstSuspect);
    const exitRegion = exit?.country ? dnsRegion(exit.country) : '';
    const note = label
      ? exitRegion && firstSuspect?.country && firstSuspect.country !== exit?.country
        ? `WebRTC 测到了归属${label}的公网 IP，与当前出口（${exitRegion}）不一致。`
        : `WebRTC 测到了归属${label}的公网 IP，与当前出口不属于同一网络。`
      : 'WebRTC 测到的公网 IP 与当前出口不属于同一网络。';
    return {
      state: 'leaked',
      reason: 'public-leak',
      note,
      suspectIPs,
    };
  }

  if (res.verdict.state === 'unknown') {
    return {
      state: 'unknown',
      reason: reason === 'missing-exit' ? 'missing-exit' : 'assessment-unavailable',
      note:
        reason === 'missing-exit'
          ? '已测到 WebRTC 公网 IP，但未获取到出口 IP 进行比对。'
          : '暂未获取到足够归属信息进行比对。',
      suspectIPs: [],
    };
  }

  switch (reason) {
    case 'same-network':
      return {
        state: 'clear',
        reason: 'same-network',
        note: 'WebRTC 测到同一运营商 ASN 下的公网出口（常见于蜂窝或运营商 CGNAT 出口池），未发现跨网泄露。',
        suspectIPs: [],
      };
    case 'dual-stack':
      return {
        state: 'clear',
        reason: 'dual-stack',
        note: 'WebRTC 测到另一协议栈的公网 IP，归属与当前出口一致，属正常 IPv4/IPv6 双栈。',
        suspectIPs: [],
      };
    case 'same-prefix':
      return {
        state: 'clear',
        reason: 'same-prefix',
        note: 'WebRTC 测到的 IPv6 地址与当前出口属于同一 /64 前缀（临时隐私地址），未发生泄露。',
        suspectIPs: [],
      };
    default:
      return {
        state: 'clear',
        reason: 'matches-exit',
        note: 'WebRTC 测到的公网 IP 与当前出口一致。',
        suspectIPs: [],
      };
  }
}

export async function measureWebRTC(
  onExit?: (exit: DNSExit) => void,
  onProgress?: (progress: WebRTCProgress) => void,
): Promise<WebRTCCheck> {
  let exit: DNSExit | null = null;
  let exitError: WebRTCCheck['exitError'];
  let phase: WebRTCPhase = 'gathering';
  let candidateCount = 0;
  let publicIPs: string[] = [];
  let localIPs: string[] = [];
  let mdnsHosts: string[] = [];
  let ipv6Global: string | null = null;
  let apiSupported = typeof RTCPeerConnection !== 'undefined';

  const emit = () => {
    onProgress?.({
      phase,
      exit,
      candidateCount,
      publicIPs,
      localIPs,
      mdnsHosts,
      ipv6Global,
      apiSupported,
    });
  };

  emit();

  const exitPromise = getBrowserExit()
    .then((info) => {
      exit = browserExit(info);
      onExit?.(exit);
      emit();
    })
    .catch((err) => {
      exitError = err instanceof ApiError && err.status === 429 ? 'limited' : 'unavailable';
    });

  const [rtc] = await Promise.all([
    gatherRTCCandidates((snap) => {
      apiSupported = snap.apiSupported;
      candidateCount = snap.candidates.length;
      publicIPs = snap.publicIPs;
      localIPs = snap.localIPs;
      mdnsHosts = snap.mdnsHosts;
      ipv6Global = snap.ipv6Global;
      if (candidateCount > 0) phase = 'collected';
      emit();
    }),
    exitPromise,
  ]);

  phase = 'comparing';
  emit();

  const canonicalPublicIPs = dedupeCanonicalIPs(rtc.publicIPs);
  const canonicalStunIPs = dedupeCanonicalIPs(rtc.stunIPs);
  const canonicalIPv6 = rtc.ipv6Global ? canonicalIP(rtc.ipv6Global) : null;
  const canonicalRTC: RTCGatherResult = {
    ...rtc,
    publicIPs: canonicalPublicIPs,
    stunIPs: canonicalStunIPs,
    ipv6Global: canonicalIPv6,
    candidates: rtc.candidates.map((cand) => ({
      ...cand,
      address: cand.kind === 'mdns' ? cand.address : canonicalIP(cand.address),
    })),
  };

  let verdict: WebRTCVerdict;
  let publicCandidates: WebRTCNetworkInfo[] | undefined;
  let resolvedExit = exit as DNSExit | null;

  const needsAttribution =
    canonicalRTC.apiSupported &&
    canonicalPublicIPs.length > 0 &&
    Boolean(resolvedExit?.ip) &&
    canonicalPublicIPs.some((ip) => !sameExitOrPrefix64(ip, resolvedExit?.ip));

  if (needsAttribution) {
    try {
      const assessed = await assessWebRTCCandidates(canonicalPublicIPs);
      publicCandidates = assessed.candidates;
      if (assessed.exit?.ip && resolvedExit && !resolvedExit.country && assessed.exit.country) {
        resolvedExit = { ...resolvedExit, country: assessed.exit.country };
      }
      verdict = verdictFromAssessment(assessed, resolvedExit);
    } catch {
      verdict = evaluateWebRTC(resolvedExit, canonicalRTC);
    }
  } else {
    verdict = evaluateWebRTC(resolvedExit, canonicalRTC);
  }

  return {
    time: Date.now(),
    exit: resolvedExit,
    apiSupported: canonicalRTC.apiSupported,
    publicIPs: canonicalRTC.publicIPs,
    ...(publicCandidates ? { publicCandidates } : {}),
    stunIPs: canonicalRTC.stunIPs,
    localIPs: canonicalRTC.localIPs,
    mdnsHosts: canonicalRTC.mdnsHosts,
    ipv6Global: canonicalRTC.ipv6Global,
    candidates: canonicalRTC.candidates,
    verdict,
    ...(exitError ? { exitError } : {}),
  };
}

export const webrtcHistory = (
  next?: WebRTCCheck,
  lastInPage?: WebRTCCheck | null,
): Promise<WebRTCHistory> => recentHistory<WebRTCCheck>('detectradar-webrtc', next, lastInPage);

const STATE_RANK: Record<WebRTCVerdictState, number> = {
  clear: 0,
  unknown: 1,
  leaked: 2,
};

function normalizeVerdictState(verdict: WebRTCVerdict): WebRTCVerdictState {
  const raw = verdict.state as string;
  if (raw === 'mismatch') {
    return verdict.reason === 'public-mismatch' || verdict.reason === 'proxy-bypass'
      ? 'leaked'
      : 'clear';
  }
  return raw === 'leaked' || raw === 'clear' || raw === 'unknown' ? raw : 'unknown';
}

function formatIPDiff(added: string[], removed: string[], fallbackEmpty: string): string {
  const parts: string[] = [];
  if (added.length) parts.push(`新增 ${added.join('、')}`);
  if (removed.length) parts.push(`不再出现 ${removed.join('、')}`);
  return parts.length ? parts.join('；') : fallbackEmpty;
}

export function compareWebRTC(previous: WebRTCCheck, current: WebRTCCheck): WebRTCComparison {
  const prevPubList = dedupeCanonicalIPs(previous.publicIPs);
  const curPubList = dedupeCanonicalIPs(current.publicIPs);
  const prevPub = new Set(prevPubList);
  const curPub = new Set(curPubList);
  const addedPublicList = curPubList.filter((ip) => !prevPub.has(ip));
  const removedPublicIPs = prevPubList.filter((ip) => !curPub.has(ip));

  const prevLocal = new Set(previous.localIPs);
  const curLocal = new Set(current.localIPs);
  const addedLocalList = current.localIPs.filter((ip) => !prevLocal.has(ip));
  const removedLocalIPs = previous.localIPs.filter((ip) => !curLocal.has(ip));

  const prevExitIP = canonicalIP(previous.exit?.ip);
  const curExitIP = canonicalIP(current.exit?.ip);
  const exitChange =
    prevExitIP && curExitIP
      ? sameIP(prevExitIP, curExitIP)
        ? '未变'
        : `从 ${prevExitIP} 变为 ${curExitIP}`
      : !prevExitIP && curExitIP
        ? `本次获取到 ${curExitIP}`
        : prevExitIP && !curExitIP
          ? '本次未获取到'
          : '未变';

  const localChange = (() => {
    if (addedLocalList.length === 0 && removedLocalIPs.length === 0) {
      if (previous.mdnsHosts.length === 0 && current.mdnsHosts.length > 0) return '转为 mDNS（*.local）保护';
      return '未变';
    }
    if (addedLocalList.length === 0 && current.localIPs.length === 0 && current.mdnsHosts.length > 0) {
      return `已隐藏原内网 IP，改用 mDNS（*.local）`;
    }
    return formatIPDiff(addedLocalList, removedLocalIPs, '未变');
  })();

  const publicChange = formatIPDiff(addedPublicList, removedPublicIPs, '未变');
  const prevState = normalizeVerdictState(previous.verdict);
  const curState = normalizeVerdictState(current.verdict);
  const previousVerdict = prevState !== curState ? prevState : undefined;
  const improved = STATE_RANK[curState] < STATE_RANK[prevState];

  const unchanged =
    !previousVerdict &&
    addedPublicList.length === 0 &&
    removedPublicIPs.length === 0 &&
    addedLocalList.length === 0 &&
    removedLocalIPs.length === 0 &&
    exitChange === '未变' &&
    previous.apiSupported === current.apiSupported;

  return {
    improved,
    previousVerdict,
    addedPublicIPs: new Set(addedPublicList),
    removedPublicIPs,
    addedLocalIPs: new Set(addedLocalList),
    removedLocalIPs,
    publicChange,
    localChange,
    exitChange,
    summary: unchanged ? 'WebRTC 地址与出口 IP 都和上次一样。' : null,
  };
}
