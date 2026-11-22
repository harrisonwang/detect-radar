import { ApiError, getBrowserExit } from './api';
import {
  gatherRTCCandidates,
  type RTCCandidateDetail,
  type RTCGatherResult,
  type RTCLocalScope,
} from './collectors/leaks/webrtc';
import { browserExit } from './dnsPresentation';
import { recentHistory, type RecentHistory } from './recentHistory';
import type { DNSExit } from './types';

export type WebRTCVerdictState = 'leaked' | 'mismatch' | 'clear' | 'unknown';

export type WebRTCVerdictReason =
  | 'proxy-bypass'
  | 'public-mismatch'
  | 'local-exposed'
  | 'matches-exit'
  | 'dual-stack'
  | 'mdns-protected'
  | 'no-candidates'
  | 'disabled'
  | 'missing-exit';

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
  mismatch: { title: '存在差异', color: 'var(--warn)', soft: 'var(--warn-soft)' },
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

function isIPv6Address(ip: string): boolean {
  return ip.includes(':');
}

function isExpectedDualStack(exitIP: string, extraPublicIPs: string[]): boolean {
  if (!exitIP || extraPublicIPs.length === 0) return false;
  if (!isIPv6Address(exitIP) && extraPublicIPs.every(isIPv6Address)) return true;
  if (isIPv6Address(exitIP) && extraPublicIPs.every((ip) => !isIPv6Address(ip))) return true;
  return false;
}

export function evaluateWebRTC(exit: DNSExit | null, rtc: RTCGatherResult): WebRTCVerdict {
  if (!rtc.apiSupported) {
    return {
      state: 'clear',
      reason: 'disabled',
      note: '浏览器已禁用 WebRTC 接口，网页无法读取候选地址（同时会跳过依赖 ICE 的 IPv6 探测）。',
      suspectIPs: [],
    };
  }

  const exitIP = exit?.ip?.trim() ?? '';
  if (rtc.publicIPs.length > 0 && !exitIP) {
    return {
      state: 'unknown',
      reason: 'missing-exit',
      note: '已获取到 WebRTC 公网候选，但未能获取当前出口 IP 进行比对。',
      suspectIPs: [],
    };
  }

  const exitHasProxyRisk = exit?.type === 'proxy' || exit?.type === 'hosting';
  const extraPublicIPs = exitIP
    ? rtc.publicIPs.filter((ip) => ip.toLowerCase() !== exitIP.toLowerCase())
    : rtc.publicIPs;

  if (extraPublicIPs.length > 0 && exitHasProxyRisk) {
    return {
      state: 'leaked',
      reason: 'proxy-bypass',
      note: '当前为代理或机房出口，WebRTC 暴露了与出口不同的公网地址，可能绕过代理直连。',
      suspectIPs: extraPublicIPs,
    };
  }

  if (extraPublicIPs.length > 0) {
    if (isExpectedDualStack(exitIP, extraPublicIPs)) {
      return {
        state: 'clear',
        reason: 'dual-stack',
        note: '测到另一协议栈的公网地址，在直连网络下属于正常 IPv4/IPv6 双栈表现。',
        suspectIPs: [],
      };
    }
    return {
      state: 'mismatch',
      reason: 'public-mismatch',
      note: 'WebRTC 公网候选与当前出口 IP 不一致；如正在使用代理或分流，请核对 UDP 转发规则。',
      suspectIPs: extraPublicIPs,
    };
  }

  if (rtc.publicIPs.length === 0 && rtc.localIPs.length > 0) {
    return {
      state: 'mismatch',
      reason: 'local-exposed',
      note: '未暴露公网地址，但网页可直接读取局域网或虚拟网卡内网地址（mDNS 未隐藏）。',
      suspectIPs: [],
    };
  }

  if (rtc.publicIPs.length > 0) {
    return {
      state: 'clear',
      reason: 'matches-exit',
      note: 'WebRTC 公网候选与当前出口 IP 一致，未发现额外的公网导出路径。',
      suspectIPs: [],
    };
  }

  if (rtc.mdnsHosts.length > 0) {
    return {
      state: 'clear',
      reason: 'mdns-protected',
      note: '浏览器仅返回 mDNS（*.local）加密主机名，未暴露公网或真实内网 IP。',
      suspectIPs: [],
    };
  }

  return {
    state: 'clear',
    reason: 'no-candidates',
    note: '本次未收集到 WebRTC 公网或内网 IP 候选。',
    suspectIPs: [],
  };
}

export async function measureWebRTC(onExit?: (exit: DNSExit) => void): Promise<WebRTCCheck> {
  let exit: DNSExit | null = null;
  let exitError: WebRTCCheck['exitError'];

  const exitPromise = getBrowserExit()
    .then((info) => {
      exit = browserExit(info);
      onExit?.(exit);
    })
    .catch((err) => {
      exitError = err instanceof ApiError && err.status === 429 ? 'limited' : 'unavailable';
    });

  const [rtc] = await Promise.all([gatherRTCCandidates(), exitPromise]);

  return {
    time: Date.now(),
    exit,
    apiSupported: rtc.apiSupported,
    publicIPs: rtc.publicIPs,
    stunIPs: rtc.stunIPs,
    localIPs: rtc.localIPs,
    mdnsHosts: rtc.mdnsHosts,
    ipv6Global: rtc.ipv6Global,
    candidates: rtc.candidates,
    verdict: evaluateWebRTC(exit, rtc),
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
  mismatch: 2,
  leaked: 3,
};

function formatIPDiff(added: string[], removed: string[], fallbackEmpty: string): string {
  const parts: string[] = [];
  if (added.length) parts.push(`新增 ${added.join('、')}`);
  if (removed.length) parts.push(`不再出现 ${removed.join('、')}`);
  return parts.length ? parts.join('；') : fallbackEmpty;
}

export function compareWebRTC(previous: WebRTCCheck, current: WebRTCCheck): WebRTCComparison {
  const prevPub = new Set(previous.publicIPs);
  const curPub = new Set(current.publicIPs);
  const addedPublicList = current.publicIPs.filter((ip) => !prevPub.has(ip));
  const removedPublicIPs = previous.publicIPs.filter((ip) => !curPub.has(ip));

  const prevLocal = new Set(previous.localIPs);
  const curLocal = new Set(current.localIPs);
  const addedLocalList = current.localIPs.filter((ip) => !prevLocal.has(ip));
  const removedLocalIPs = previous.localIPs.filter((ip) => !curLocal.has(ip));

  const exitChange =
    previous.exit?.ip && current.exit?.ip
      ? previous.exit.ip === current.exit.ip
        ? '未变'
        : `从 ${previous.exit.ip} 变为 ${current.exit.ip}`
      : !previous.exit?.ip && current.exit?.ip
        ? `本次获取到 ${current.exit.ip}`
        : previous.exit?.ip && !current.exit?.ip
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
  const previousVerdict =
    previous.verdict.state !== current.verdict.state ? previous.verdict.state : undefined;
  const improved = STATE_RANK[current.verdict.state] < STATE_RANK[previous.verdict.state];

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
    summary: unchanged ? 'WebRTC 候选与出口 IP 都和上次一样。' : null,
  };
}
