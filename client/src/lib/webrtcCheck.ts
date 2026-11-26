import { getMessages, joinList, type Locale } from '../i18n';
import { ApiError, assessWebRTCCandidates, getBrowserExit } from './api';
import {
  gatherRTCCandidates,
  type RTCCandidateDetail,
  type RTCGatherResult,
  type RTCLocalScope,
} from './collectors/leaks/webrtc';
import { browserExit, dnsProvider, dnsRegion } from './dnsPresentation';
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
  | 'reverse-split'
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
  locale?: Locale;
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

export function getWebrtcStatus(locale: Locale = 'zh-CN'): Record<
  WebRTCVerdictState,
  { title: string; color: string; soft: string }
> {
  const s = getMessages(locale).webrtc.status;
  return {
    leaked: { title: s.leaked, color: 'var(--leak)', soft: 'var(--leak-soft)' },
    clear: { title: s.clear, color: 'var(--sig)', soft: 'var(--sig-soft)' },
    unknown: { title: s.unknown, color: 'var(--unknown)', soft: 'var(--unknown-soft)' },
  };
}

export const webrtcStatus = getWebrtcStatus('zh-CN');

export function localScopeLabel(scope?: RTCLocalScope, locale: Locale = 'zh-CN'): string {
  const s = getMessages(locale).webrtc.scopes;
  return scope ? s[scope] : s.fallback;
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

export function evaluateWebRTC(
  exit: DNSExit | null,
  rtc: RTCGatherResult,
  locale: Locale = 'zh-CN',
): WebRTCVerdict {
  const n = getMessages(locale).webrtc.verdictNote;
  if (!rtc.apiSupported) {
    return { state: 'clear', reason: 'disabled', note: n.disabled, suspectIPs: [] };
  }

  const publicIPs = dedupeCanonicalIPs(rtc.publicIPs);
  const exitIP = canonicalIP(exit?.ip);
  if (publicIPs.length > 0 && !exitIP) {
    return { state: 'unknown', reason: 'missing-exit', note: n.missingExit, suspectIPs: [] };
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
          note: n.assessmentUnavailableDual,
          suspectIPs: [],
        };
      }
      return { state: 'clear', reason: 'dual-stack', note: n.dualStackGeneric, suspectIPs: [] };
    }
    return {
      state: 'leaked',
      reason: 'public-leak',
      note: n.publicLeakGeneric,
      suspectIPs: extraPublicIPs,
    };
  }

  if (publicIPs.length > 0 && exitIP && publicIPs.some((ip) => !sameIP(ip, exitIP) && sameIPv6Prefix64(ip, exitIP))) {
    return { state: 'clear', reason: 'same-prefix', note: n.samePrefix, suspectIPs: [] };
  }

  if (publicIPs.length === 0 && rtc.localIPs.length > 0) {
    return { state: 'clear', reason: 'local-exposed', note: n.localExposed, suspectIPs: [] };
  }

  if (publicIPs.length > 0) {
    return { state: 'clear', reason: 'matches-exit', note: n.matchesExit, suspectIPs: [] };
  }

  if (rtc.mdnsHosts.length > 0) {
    return { state: 'clear', reason: 'mdns-protected', note: n.mdnsProtected, suspectIPs: [] };
  }

  return { state: 'clear', reason: 'no-candidates', note: n.noCandidates, suspectIPs: [] };
}

function candidateLabelParts(
  info: WebRTCNetworkInfo | undefined,
  locale: Locale,
): { region: string; provider: string } {
  if (!info) return { region: '', provider: '' };
  const unknownRegion = getMessages(locale).common.unknownRegion;
  const region = info.country ? dnsRegion(info.country, locale) : '';
  const validRegion = region && region !== unknownRegion ? region : '';
  const provider = dnsProvider(info.network ?? '');
  return { region: validRegion, provider };
}

export function webrtcVerdictNote(
  check: Pick<WebRTCCheck, 'verdict' | 'exit' | 'publicCandidates'>,
  locale: Locale = 'zh-CN',
): string {
  const n = getMessages(locale).webrtc.verdictNote;
  const { verdict, exit, publicCandidates } = check;
  switch (verdict.reason) {
    case 'disabled':
      return n.disabled;
    case 'missing-exit':
      return n.missingExit;
    case 'assessment-unavailable':
      return publicCandidates?.length ? n.assessmentUnavailableWithCands : n.assessmentUnavailableDual;
    case 'reverse-split': {
      const firstSuspect = publicCandidates?.find((c) => c.relation === 'leak');
      const { region, provider } = candidateLabelParts(firstSuspect, locale);
      const exitRegion = exit?.country ? dnsRegion(exit.country, locale) : '';
      if (region || provider) {
        return n.reverseSplit(region, provider, exitRegion);
      }
      return n.reverseSplitGeneric;
    }
    case 'public-leak': {
      const firstSuspect = publicCandidates?.find((c) => c.relation === 'leak');
      const { region, provider } = candidateLabelParts(firstSuspect, locale);
      const exitRegion = exit?.country ? dnsRegion(exit.country, locale) : '';
      if (region || provider) {
        return exitRegion && firstSuspect?.country && firstSuspect.country !== exit?.country
          ? n.publicLeakDiffCountry(region, provider, exitRegion)
          : n.publicLeakDiffNet(region, provider);
      }
      return publicCandidates?.length ? n.publicLeakGenericNet : n.publicLeakGeneric;
    }
    case 'same-network':
      return n.sameNetwork;
    case 'dual-stack':
      return publicCandidates?.length ? n.dualStackSameISP : n.dualStackGeneric;
    case 'same-prefix':
      return n.samePrefix;
    case 'local-exposed':
      return n.localExposed;
    case 'matches-exit':
      return n.matchesExit;
    case 'mdns-protected':
      return n.mdnsProtected;
    case 'no-candidates':
      return n.noCandidates;
    default:
      return verdict.note || '';
  }
}

export function verdictFromAssessment(
  res: WebRTCAssessmentResponse,
  exit: DNSExit | null,
  locale: Locale = 'zh-CN',
): WebRTCVerdict {
  const n = getMessages(locale).webrtc.verdictNote;
  const suspectIPs = dedupeCanonicalIPs(res.verdict.suspect_ips ?? []);
  const reason = (res.verdict.reason || 'matches-exit') as WebRTCVerdictReason;

  if (res.verdict.state === 'leaked') {
    const firstSuspect = res.candidates.find((c) => c.relation === 'leak');
    const { region, provider } = candidateLabelParts(firstSuspect, locale);
    const exitRegion = exit?.country ? dnsRegion(exit.country, locale) : '';
    if (reason === 'reverse-split') {
      const note = region || provider
        ? n.reverseSplit(region, provider, exitRegion)
        : n.reverseSplitGeneric;
      return { state: 'leaked', reason: 'reverse-split', note, suspectIPs };
    }
    const note = region || provider
      ? exitRegion && firstSuspect?.country && firstSuspect.country !== exit?.country
        ? n.publicLeakDiffCountry(region, provider, exitRegion)
        : n.publicLeakDiffNet(region, provider)
      : n.publicLeakGenericNet;
    return { state: 'leaked', reason: 'public-leak', note, suspectIPs };
  }

  if (res.verdict.state === 'unknown') {
    return {
      state: 'unknown',
      reason: reason === 'missing-exit' ? 'missing-exit' : 'assessment-unavailable',
      note: reason === 'missing-exit' ? n.missingExit : n.assessmentUnavailableWithCands,
      suspectIPs: [],
    };
  }

  switch (reason) {
    case 'same-network':
      return { state: 'clear', reason: 'same-network', note: n.sameNetwork, suspectIPs: [] };
    case 'dual-stack':
      return { state: 'clear', reason: 'dual-stack', note: n.dualStackSameISP, suspectIPs: [] };
    case 'same-prefix':
      return { state: 'clear', reason: 'same-prefix', note: n.samePrefix, suspectIPs: [] };
    default:
      return { state: 'clear', reason: 'matches-exit', note: n.matchesExit, suspectIPs: [] };
  }
}

export async function measureWebRTC(
  onExit?: (exit: DNSExit) => void,
  onProgress?: (progress: WebRTCProgress) => void,
  locale: Locale = 'zh-CN',
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

  const exitPromise = getBrowserExit(locale)
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
      const assessed = await assessWebRTCCandidates(canonicalPublicIPs, locale);
      publicCandidates = assessed.candidates;
      if (assessed.exit?.ip && resolvedExit) {
        resolvedExit = {
          ...resolvedExit,
          country: resolvedExit.country || assessed.exit.country || '',
          network: assessed.exit.network || resolvedExit.network,
        };
      }
      verdict = verdictFromAssessment(assessed, resolvedExit, locale);
    } catch {
      verdict = evaluateWebRTC(resolvedExit, canonicalRTC, locale);
    }
  } else {
    verdict = evaluateWebRTC(resolvedExit, canonicalRTC, locale);
  }

  return {
    time: Date.now(),
    locale,
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

function formatIPDiff(
  added: string[],
  removed: string[],
  fallbackEmpty: string,
  locale: Locale,
): string {
  const c = getMessages(locale).webrtc.compare;
  const parts: string[] = [];
  if (added.length) parts.push(c.added(joinList(added, locale)));
  if (removed.length) parts.push(c.removed(joinList(removed, locale)));
  return parts.length ? parts.join(locale === 'en' ? '; ' : '；') : fallbackEmpty;
}

export function compareWebRTC(
  previous: WebRTCCheck,
  current: WebRTCCheck,
  locale: Locale = 'zh-CN',
): WebRTCComparison {
  const m = getMessages(locale);
  const c = m.webrtc.compare;
  const unchangedLabel = m.common.unchanged;

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
        ? unchangedLabel
        : c.exitChanged(prevExitIP, curExitIP)
      : !prevExitIP && curExitIP
        ? c.exitAcquired(curExitIP)
        : prevExitIP && !curExitIP
          ? c.exitLost
          : unchangedLabel;

  const localChange = (() => {
    if (addedLocalList.length === 0 && removedLocalIPs.length === 0) {
      if (previous.mdnsHosts.length === 0 && current.mdnsHosts.length > 0) return c.mdnsSwitched;
      return unchangedLabel;
    }
    if (addedLocalList.length === 0 && current.localIPs.length === 0 && current.mdnsHosts.length > 0) {
      return c.mdnsReplacedLocal;
    }
    return formatIPDiff(addedLocalList, removedLocalIPs, unchangedLabel, locale);
  })();

  const publicChange = formatIPDiff(addedPublicList, removedPublicIPs, unchangedLabel, locale);
  const prevState = previous.verdict.state;
  const curState = current.verdict.state;
  const previousVerdict = prevState !== curState ? prevState : undefined;
  const improved = STATE_RANK[curState] < STATE_RANK[prevState];

  const exitUnchanged =
    (!prevExitIP && !curExitIP) || (Boolean(prevExitIP && curExitIP) && sameIP(prevExitIP, curExitIP));

  const unchanged =
    !previousVerdict &&
    addedPublicList.length === 0 &&
    removedPublicIPs.length === 0 &&
    addedLocalList.length === 0 &&
    removedLocalIPs.length === 0 &&
    exitUnchanged &&
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
    summary: unchanged ? c.identical : null,
  };
}
