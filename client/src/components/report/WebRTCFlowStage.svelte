<script lang="ts">
  import { onDestroy } from 'svelte';
  import { getMessages, type Locale } from '../../i18n';
  import { dnsProviderName, dnsRegion } from '../../lib/dnsPresentation';
  import { maskIP } from '../../lib/ipDisplay';
  import type { DNSExit } from '../../lib/types';
  import type { WebRTCProgress, WebRTCVerdictReason, WebRTCVerdictState } from '../../lib/webrtcCheck';

  type NodeId = 'browser' | 'stun' | 'exit' | 'judge';
  type Pt = readonly [number, number];

  interface Props {
    locale?: Locale;
    exit: DNSExit | null;
    verdictState: WebRTCVerdictState;
    verdictReason?: WebRTCVerdictReason;
    verdictNote?: string;
    apiSupported: boolean;
    publicIPs: string[];
    suspectIPs: string[];
    localIPs: string[];
    mdnsHosts: string[];
    revealExit: boolean;
    busy?: boolean;
    liveProgress?: WebRTCProgress | null;
    hasResult?: boolean;
  }

  let {
    locale = 'zh-CN',
    exit,
    verdictState,
    verdictReason,
    verdictNote = '',
    apiSupported,
    publicIPs,
    suspectIPs,
    localIPs,
    mdnsHosts,
    revealExit,
    busy = false,
    liveProgress = null,
    hasResult = false,
  }: Props = $props();

  let m = $derived(getMessages(locale));
  let fl = $derived(m.webrtc.flow);
  let dnsFl = $derived(m.dns.flow);

  let httpsT = $state(1);
  let httpsActive = $state(false);
  let stunT = $state(1);
  let stunActive = $state(false);
  let verdictPulseT = $state(1);
  let verdictPulsing = $state(false);
  let selectedNode = $state<NodeId | null>(null);

  let rafId = 0;
  let hadLiveScan = false;
  let lastBusy = false;
  let scanStartAt = 0;

  let httpsPhase: 'idle' | 'probing' | 'arriving' | 'finishing' | 'docked' = 'idle';
  let httpsArrivedAt = 0;
  let httpsFromT = 0;

  let stunPhase: 'idle' | 'gathering' | 'collected' | 'comparing' | 'finishing' | 'docked' = 'idle';
  let stunPhaseStartAt = 0;
  let stunFromT = 0;

  let pendingVerdictRipple = false;
  let verdictRippleStartAt = 0;

  const DESKTOP_STUN: readonly Pt[] = [
    [106, 148],
    [186, 84],
    [380, 84],
    [574, 84],
    [654, 148],
  ];
  const DESKTOP_STUN_HALT: readonly Pt[] = [
    [106, 148],
    [186, 84],
    [318, 84],
  ];
  const DESKTOP_HTTPS: readonly Pt[] = [
    [106, 148],
    [186, 212],
    [380, 212],
    [574, 212],
    [654, 148],
  ];

  const MOBILE_STUN: readonly Pt[] = [
    [170, 42],
    [78, 42],
    [78, 152],
    [166, 152],
    [166, 254],
  ];
  const MOBILE_STUN_HALT: readonly Pt[] = [
    [170, 42],
    [78, 42],
    [78, 96],
  ];
  const MOBILE_HTTPS: readonly Pt[] = [
    [170, 42],
    [262, 42],
    [262, 152],
    [174, 152],
    [174, 254],
  ];

  const NODE_POS_DESKTOP: Record<NodeId, Pt> = {
    browser: [106, 148],
    stun: [380, 84],
    exit: [380, 212],
    judge: [654, 148],
  };

  const NODE_POS_MOBILE: Record<NodeId, Pt> = {
    browser: [170, 42],
    stun: [78, 152],
    exit: [262, 152],
    judge: [170, 254],
  };

  function pillWidth(label: string, minW = 38): number {
    let w = 16;
    for (const ch of label) {
      w += /[\u3000-\u9fff]/.test(ch) ? 10 : 6.1;
    }
    return Math.max(minW, Math.ceil(w));
  }

  function pointAlong(pts: readonly Pt[], rawT: number): { x: number; y: number } {
    const t = Math.max(0, Math.min(1, rawT));
    if (pts.length === 0) return { x: 0, y: 0 };
    if (pts.length === 1 || t <= 0) return { x: pts[0][0], y: pts[0][1] };
    if (t >= 1) return { x: pts[pts.length - 1][0], y: pts[pts.length - 1][1] };

    const lens: number[] = [];
    let total = 0;
    for (let i = 1; i < pts.length; i++) {
      const dx = pts[i][0] - pts[i - 1][0];
      const dy = pts[i][1] - pts[i - 1][1];
      const len = Math.hypot(dx, dy);
      lens.push(len);
      total += len;
    }
    if (total === 0) return { x: pts[0][0], y: pts[0][1] };

    let target = t * total;
    for (let i = 0; i < lens.length; i++) {
      if (target <= lens[i] || i === lens.length - 1) {
        const local = lens[i] > 0 ? target / lens[i] : 0;
        return {
          x: pts[i][0] + (pts[i + 1][0] - pts[i][0]) * local,
          y: pts[i][1] + (pts[i + 1][1] - pts[i][1]) * local,
        };
      }
      target -= lens[i];
    }
    return { x: pts[pts.length - 1][0], y: pts[pts.length - 1][1] };
  }

  function polyToD(pts: readonly Pt[]): string {
    return pts.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p[0]} ${p[1]}`).join(' ');
  }

  function easeInOut(t: number): number {
    const x = Math.max(0, Math.min(1, t));
    return x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2;
  }

  function easeOut(t: number): number {
    const x = Math.max(0, Math.min(1, t));
    return 1 - Math.pow(1 - x, 3);
  }

  function prefersReducedMotion(): boolean {
    return (
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    );
  }

  function stopAnim() {
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = 0;
    }
  }

  function stepAnimation(now: number) {
    let keepGoing = false;

    if (httpsPhase === 'probing') {
      const u = Math.min(1, (now - scanStartAt) / 850);
      httpsT = easeOut(u) * 0.42;
      httpsActive = true;
      keepGoing = true;
    } else if (httpsPhase === 'arriving') {
      const u = Math.min(1, (now - httpsArrivedAt) / 1100);
      const mid = Math.max(httpsFromT, 0.5);
      if (u < 0.42) {
        httpsT = httpsFromT + easeInOut(u / 0.42) * (mid - httpsFromT);
      } else if (u < 0.58) {
        httpsT = mid;
      } else {
        httpsT = mid + easeInOut((u - 0.58) / 0.42) * (1 - mid);
      }
      if (u >= 1) {
        httpsT = 1;
        httpsActive = false;
        httpsPhase = 'docked';
      } else {
        httpsActive = true;
        keepGoing = true;
      }
    } else if (httpsPhase === 'finishing') {
      const u = Math.min(1, (now - httpsArrivedAt) / 700);
      httpsT = httpsFromT + easeInOut(u) * (1 - httpsFromT);
      if (u >= 1) {
        httpsT = 1;
        httpsActive = false;
        httpsPhase = 'docked';
      } else {
        httpsActive = true;
        keepGoing = true;
      }
    }

    if (stunPhase === 'gathering') {
      stunActive = true;
      const dur = 2000;
      const cycle = ((now - scanStartAt) % dur) / dur;
      if (cycle < 0.72) {
        stunT = easeInOut(cycle / 0.72) * 0.51;
      } else {
        stunT = 0.51 - Math.sin(((cycle - 0.72) / 0.28) * Math.PI) * 0.025;
      }
      keepGoing = true;
    } else if (stunPhase === 'collected') {
      const u = Math.min(1, (now - stunPhaseStartAt) / 1000);
      stunT = stunFromT + easeInOut(u) * (1 - stunFromT);
      if (u >= 1) {
        stunT = 1;
        stunActive = false;
        stunPhase = 'docked';
      } else {
        stunActive = true;
        keepGoing = true;
      }
    } else if (stunPhase === 'finishing') {
      const u = Math.min(1, (now - stunPhaseStartAt) / 750);
      stunT = stunFromT + easeInOut(u) * (1 - stunFromT);
      if (u >= 1) {
        stunT = 1;
        stunActive = false;
        stunPhase = 'docked';
      } else {
        stunActive = true;
        keepGoing = true;
      }
    }

    if (pendingVerdictRipple && httpsPhase === 'docked' && stunPhase === 'docked') {
      pendingVerdictRipple = false;
      verdictPulsing = true;
      verdictRippleStartAt = now;
      verdictPulseT = 0;
      keepGoing = true;
    }

    if (verdictPulsing) {
      const u = Math.min(1, (now - verdictRippleStartAt) / 1000);
      verdictPulseT = easeOut(u);
      if (u >= 1) {
        verdictPulsing = false;
        verdictPulseT = 1;
      } else {
        keepGoing = true;
      }
    }

    if (keepGoing) {
      rafId = requestAnimationFrame(stepAnimation);
    } else {
      rafId = 0;
    }
  }

  function ensureRaf() {
    if (!rafId) {
      rafId = requestAnimationFrame(stepAnimation);
    }
  }

  onDestroy(stopAnim);

  let activeExit = $derived(busy && liveProgress ? liveProgress.exit : exit);
  let activeApiSupported = $derived(
    busy && liveProgress ? liveProgress.apiSupported : apiSupported,
  );
  let activePublicIPs = $derived(busy && liveProgress ? liveProgress.publicIPs : publicIPs);
  let activeLocalIPs = $derived(busy && liveProgress ? liveProgress.localIPs : localIPs);
  let activeMdnsHosts = $derived(busy && liveProgress ? liveProgress.mdnsHosts : mdnsHosts);
  let activeCandidateCount = $derived(
    busy && liveProgress
      ? liveProgress.candidateCount
      : publicIPs.length + localIPs.length + mdnsHosts.length,
  );

  let effectiveVerdict = $derived<WebRTCVerdictState>(busy ? 'unknown' : verdictState);
  let isHaltedBranch = $derived(
    !busy && (!activeApiSupported || verdictReason === 'no-candidates'),
  );

  $effect(() => {
    const reduced = prefersReducedMotion();
    const hasLiveExit = Boolean(activeExit?.ip);
    const livePhase = liveProgress?.phase ?? 'gathering';

    if (busy) {
      if (!lastBusy) {
        lastBusy = true;
        hadLiveScan = true;
        scanStartAt = performance.now();
        pendingVerdictRipple = false;
        verdictPulsing = false;
        verdictPulseT = 1;

        if (reduced) {
          httpsPhase = hasLiveExit ? 'docked' : 'probing';
          httpsT = hasLiveExit ? 1 : 0.42;
          httpsActive = false;
          stunPhase = livePhase;
          stunT = livePhase === 'gathering' ? 0.5 : 1;
          stunActive = false;
          stopAnim();
          return;
        }

        httpsPhase = hasLiveExit ? 'arriving' : 'probing';
        httpsArrivedAt = scanStartAt;
        httpsFromT = 0;
        httpsT = 0;
        httpsActive = true;

        stunPhase = livePhase === 'comparing' ? 'docked' : livePhase;
        stunPhaseStartAt = scanStartAt;
        stunFromT = 0;
        stunT = 0;
        stunActive = stunPhase !== 'docked';
        ensureRaf();
      } else if (reduced) {
        if (hasLiveExit) {
          httpsPhase = 'docked';
          httpsT = 1;
        }
        stunT = livePhase === 'gathering' ? 0.5 : 1;
      } else {
        if (hasLiveExit && httpsPhase === 'probing') {
          httpsPhase = 'arriving';
          httpsArrivedAt = performance.now();
          httpsFromT = httpsT;
          ensureRaf();
        }
        if (livePhase === 'collected' && stunPhase === 'gathering') {
          stunPhase = 'collected';
          stunPhaseStartAt = performance.now();
          stunFromT = Math.max(stunT, 0.48);
          stunActive = true;
          ensureRaf();
        } else if (livePhase === 'comparing') {
          if (stunPhase === 'gathering') {
            stunPhase = 'collected';
            stunPhaseStartAt = performance.now();
            stunFromT = Math.max(stunT, 0.48);
            stunActive = true;
          }
          if (httpsPhase === 'probing') {
            httpsPhase = 'arriving';
            httpsArrivedAt = performance.now();
            httpsFromT = httpsT;
          }
          ensureRaf();
        }
      }
      return;
    }

    lastBusy = false;

    if (hasResult && hadLiveScan) {
      hadLiveScan = false;
      pendingVerdictRipple = false;
      if (reduced) {
        httpsPhase = 'docked';
        httpsT = 1;
        httpsActive = false;
        stunPhase = 'docked';
        stunT = 1;
        stunActive = false;
        verdictPulsing = false;
        verdictPulseT = 1;
        stopAnim();
      } else {
        const now = performance.now();
        if (httpsActive && httpsT < 0.98) {
          httpsPhase = 'finishing';
          httpsArrivedAt = now;
          httpsFromT = httpsT;
        } else {
          httpsPhase = 'docked';
          httpsT = 1;
          httpsActive = false;
        }
        if (stunActive && !isHaltedBranch && stunT < 0.98) {
          stunPhase = 'finishing';
          stunPhaseStartAt = now;
          stunFromT = stunT;
        } else {
          stunPhase = 'docked';
          stunT = 1;
          stunActive = false;
        }
        pendingVerdictRipple = true;
        ensureRaf();
      }
    } else if (!hasResult || (!pendingVerdictRipple && !verdictPulsing && httpsPhase !== 'finishing' && stunPhase !== 'finishing')) {
      hadLiveScan = false;
      pendingVerdictRipple = false;
      httpsPhase = 'docked';
      httpsT = 1;
      httpsActive = false;
      stunPhase = 'docked';
      stunT = 1;
      stunActive = false;
      verdictPulsing = false;
      verdictPulseT = 1;
      stopAnim();
    }
  });

  function toggleNode(id: NodeId) {
    selectedNode = selectedNode === id ? null : id;
  }

  function handleNodeKey(e: KeyboardEvent, id: NodeId) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      toggleNode(id);
    }
  }

  function truncateText(str: string, max: number): string {
    return str.length > max ? `${str.slice(0, max - 1)}…` : str;
  }

  function formatCandidateIP(ip: string): string {
    return revealExit ? ip : maskIP(ip);
  }

  let isIdle = $derived(!busy && !hasResult);

  let exitName = $derived(
    activeExit?.ip
      ? `${dnsProviderName(activeExit.network, locale) || dnsFl.exitDefault} · ${dnsRegion(activeExit.country, locale)}`
      : isIdle
        ? dnsFl.exitIdle
        : dnsFl.exitFetching,
  );
  let exitDisplayIP = $derived(
    activeExit?.ip ? (revealExit ? activeExit.ip : maskIP(activeExit.ip)) : isIdle ? m.common.idle : '…',
  );

  let stunTitle = $derived.by(() => {
    if (isIdle) return fl.titleProbe;
    if (busy) {
      if (activePublicIPs.length > 0) return fl.titlePublic;
      if (activeLocalIPs.length > 0) return fl.titleLocal;
      if (activeMdnsHosts.length > 0) return fl.titleMdns;
      return fl.titleProbe;
    }

    switch (verdictReason) {
      case 'disabled':
        return fl.titleDisabled;
      case 'no-candidates':
        return fl.titleNoCands;
      case 'mdns-protected':
        return fl.titleMdns;
      case 'local-exposed':
        return fl.titleLocal;
      case 'dual-stack':
        return fl.titlePublic;
      case 'same-prefix':
        return fl.titleIPv6;
      case 'same-network':
        return fl.titleSameNet;
      case 'public-leak':
      case 'matches-exit':
      case 'missing-exit':
      case 'assessment-unavailable':
      default:
        return fl.titlePublic;
    }
  });

  let stunSub = $derived.by(() => {
    if (isIdle) return m.common.idle;
    if (busy) {
      const firstPub = activePublicIPs[0];
      if (firstPub) return formatCandidateIP(firstPub);
      const firstLocal = activeLocalIPs[0];
      if (firstLocal) return formatCandidateIP(firstLocal);
      if (activeCandidateCount > 0) return fl.subCount(activeCandidateCount);
      return fl.subGathering;
    }

    switch (verdictReason) {
      case 'disabled':
        return fl.subDisabled;
      case 'no-candidates':
        return fl.subNoCands;
      case 'mdns-protected':
        return fl.subMdns(activeMdnsHosts.length);
      case 'local-exposed': {
        const ip = activeLocalIPs[0];
        if (!ip) return fl.subLocalFallback;
        return activeLocalIPs.length > 1
          ? fl.subLocalMore(formatCandidateIP(ip), activeLocalIPs.length)
          : fl.subLocalSingle(formatCandidateIP(ip));
      }
      case 'public-leak': {
        const ip = suspectIPs[0] ?? activePublicIPs[0];
        if (!ip) return fl.subLeakFallback;
        return suspectIPs.length > 1
          ? fl.subLeakMore(formatCandidateIP(ip), suspectIPs.length)
          : fl.subLeakSingle(formatCandidateIP(ip));
      }
      case 'dual-stack': {
        const ip = activePublicIPs[0];
        return fl.subDual(ip ? formatCandidateIP(ip) : undefined);
      }
      case 'same-prefix': {
        const ip = activePublicIPs[0];
        return fl.subPrefix(ip ? formatCandidateIP(ip) : undefined);
      }
      case 'same-network': {
        const ip = activePublicIPs[0];
        return fl.subSameNet(ip ? formatCandidateIP(ip) : undefined);
      }
      case 'assessment-unavailable': {
        const ip = activePublicIPs[0];
        return fl.subUnavail(ip ? formatCandidateIP(ip) : undefined);
      }
      case 'missing-exit': {
        const ip = activePublicIPs[0];
        return ip ? formatCandidateIP(ip) : fl.subMissingExit;
      }
      case 'matches-exit':
      default: {
        const ip = activePublicIPs[0] ?? activeLocalIPs[0];
        return fl.subMatch(ip ? formatCandidateIP(ip) : undefined);
      }
    }
  });

  let stunSubCompact = $derived.by(() => {
    if (isIdle) return m.common.idle;
    if (busy) return stunSub;
    switch (verdictReason) {
      case 'mdns-protected':
        return fl.subMdnsCompact(activeMdnsHosts.length);
      case 'local-exposed':
        return activeLocalIPs[0] ? formatCandidateIP(activeLocalIPs[0]) : fl.subLocalCompactFallback;
      case 'public-leak': {
        const ip = suspectIPs[0] ?? activePublicIPs[0];
        return ip ? formatCandidateIP(ip) : fl.subLeakFallback;
      }
      case 'dual-stack':
      case 'same-prefix':
      case 'same-network':
      case 'assessment-unavailable':
      case 'missing-exit':
      case 'matches-exit':
      default: {
        const ip = activePublicIPs[0] ?? activeLocalIPs[0];
        return ip ? formatCandidateIP(ip) : stunSub;
      }
    }
  });

  let stunSubIsIP = $derived(/^(?:\d{1,3}\.){2}|^[0-9a-fA-F]{1,4}:/.test(stunSub));

  let stunAccent = $derived(
    busy
      ? liveProgress?.phase === 'collected' || liveProgress?.phase === 'comparing'
        ? '#38bdf8'
        : 'var(--warn)'
      : effectiveVerdict === 'leaked'
        ? 'var(--leak)'
        : isHaltedBranch
          ? 'var(--unknown)'
          : effectiveVerdict === 'clear'
            ? 'var(--sig)'
            : 'var(--unknown)',
  );

  let judgeAccent = $derived(
    busy
      ? stunAccent
      : effectiveVerdict === 'leaked'
        ? 'var(--leak)'
        : effectiveVerdict === 'clear'
          ? 'var(--sig)'
          : 'var(--unknown)',
  );

  let activeStunDesktopPts = $derived(isHaltedBranch ? DESKTOP_STUN_HALT : DESKTOP_STUN);
  let activeStunMobilePts = $derived(isHaltedBranch ? MOBILE_STUN_HALT : MOBILE_STUN);

  let stunPosDesktop = $derived(pointAlong(activeStunDesktopPts, stunT));
  let stunTrail1Desktop = $derived(pointAlong(activeStunDesktopPts, stunT - 0.028));
  let stunTrail2Desktop = $derived(pointAlong(activeStunDesktopPts, stunT - 0.054));

  let httpsPosDesktop = $derived(pointAlong(DESKTOP_HTTPS, httpsT));
  let httpsTrail1Desktop = $derived(pointAlong(DESKTOP_HTTPS, httpsT - 0.028));
  let httpsTrail2Desktop = $derived(pointAlong(DESKTOP_HTTPS, httpsT - 0.054));

  let stunPosMobile = $derived(pointAlong(activeStunMobilePts, stunT));
  let stunTrail1Mobile = $derived(pointAlong(activeStunMobilePts, stunT - 0.028));
  let stunTrail2Mobile = $derived(pointAlong(activeStunMobilePts, stunT - 0.054));

  let httpsPosMobile = $derived(pointAlong(MOBILE_HTTPS, httpsT));
  let httpsTrail1Mobile = $derived(pointAlong(MOBILE_HTTPS, httpsT - 0.028));
  let httpsTrail2Mobile = $derived(pointAlong(MOBILE_HTTPS, httpsT - 0.054));

  let showStunRipple = $derived(
    busy && (liveProgress?.phase === 'collected' || (!isHaltedBranch && stunT >= 0.44)),
  );
  let showExitRipple = $derived(busy && Boolean(activeExit?.ip) && httpsT >= 0.44);
  let showJudgeRipple = $derived(
    (!busy && hasResult && !pendingVerdictRipple) ||
    (busy && (liveProgress?.phase === 'comparing' || (liveProgress?.phase === 'collected' && stunT > 0.94))),
  );

  let verdictTitleText = $derived(
    isIdle
      ? m.common.idle
      : busy
        ? liveProgress?.phase === 'comparing'
          ? fl.verdictComparing
          : fl.verdictBusy
        : m.webrtc.status[effectiveVerdict],
  );

  let inspectorInfo = $derived.by(() => {
    if (!selectedNode) return null;
    switch (selectedNode) {
      case 'browser':
        return fl.inspectorBrowser;
      case 'stun':
        return fl.inspectorStun(stunTitle, stunSub);
      case 'exit':
        return fl.inspectorExit(exitName, exitDisplayIP, activeExit?.asn);
      case 'judge': {
        const cleanNote = verdictNote ? verdictNote.replace(/[。；.;]+$/, '') : '';
        return fl.inspectorJudge(verdictTitleText, cleanNote && !busy ? cleanNote : '');
      }
    }
  });

  let statusText = $derived.by(() => {
    if (!busy) return '';
    if (!liveProgress || liveProgress.phase === 'gathering') {
      return fl.statusGathering;
    }
    if (liveProgress.phase === 'collected') {
      return fl.statusCollected(liveProgress.candidateCount);
    }
    return fl.statusComparing;
  });

  let ariaSummary = $derived(
    isIdle
      ? fl.ariaIdle
      : busy && !hasResult
        ? fl.ariaBusy
        : fl.ariaResult(stunTitle, stunSub, exitName, exitDisplayIP, verdictTitleText),
  );

  let wireStunLabel = $derived(fl.wireStun);
  let wireReadLabel = $derived(isHaltedBranch ? fl.wireNoReturn : fl.wireLocalRead);
  let wireStunW = $derived(pillWidth(wireStunLabel, 44));
  let wireReadW = $derived(pillWidth(wireReadLabel, 48));
</script>

{#snippet isoNodes(prefix: string, pos: Record<NodeId, Pt>, compact: boolean)}
  {@const b = pos.browser}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.browserAria}
    onclick={() => toggleNode('browser')}
    onkeydown={(e) => handleNodeKey(e, 'browser')}
    transform={`translate(${b[0]}, ${b[1]})`}
  >
    <ellipse cx="0" cy="18" rx="30" ry="11" fill="rgba(0,0,0,0.48)" />
    {#if selectedNode === 'browser'}
      <ellipse cx="0" cy="14" rx="35" ry="15" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon points="-24,6 0,-6 26,6 2,18" fill={`url(#${prefix}-top)`} stroke="#475569" stroke-width="0.9" />
    <polygon points="-24,6 2,18 2,22 -24,10" fill={`url(#${prefix}-left)`} stroke="#334155" stroke-width="0.8" />
    <polygon points="2,18 26,6 26,10 2,22" fill={`url(#${prefix}-right)`} stroke="#1e293b" stroke-width="0.8" />
    <polygon points="-13,6 0,0 13,6 0,12" fill="rgba(15,23,42,0.72)" />
    <polygon points="-24,6 -24,-20 0,-32 0,-6" fill="#0f172a" stroke="#64748b" stroke-width="0.9" />
    <polygon points="-21,3 -21,-17 -3,-26 -3,-6" fill={`url(#${prefix}-screen)`} />
    <line x1="-18" y1="-12" x2="-7" y2="-17" stroke="var(--radar-400)" stroke-width="1.3" stroke-linecap="round" opacity="0.9" />
    <line x1="-18" y1="-8" x2="-10" y2="-12" stroke="#94a3b8" stroke-width="1.1" stroke-linecap="round" opacity="0.65" />
    <circle cx="0" cy="14" r="28" fill="transparent" />
    <text x="0" y={compact ? 34 : 38} text-anchor="middle" fill="#e2e8f0" font-size={compact ? 11.5 : 12} font-weight="600">{fl.browserNode}</text>
  </g>

  {@const st = pos.stun}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.stunAria}
    onclick={() => toggleNode('stun')}
    onkeydown={(e) => handleNodeKey(e, 'stun')}
    transform={`translate(${st[0]}, ${st[1]})`}
  >
    <ellipse
      cx="0"
      cy="20"
      rx="30"
      ry="11"
      fill={!busy && effectiveVerdict === 'leaked'
        ? 'rgba(248, 113, 113, 0.18)'
        : !busy && effectiveVerdict === 'clear' && !isHaltedBranch
          ? 'rgba(74, 222, 128, 0.14)'
          : busy && activeCandidateCount > 0
            ? 'rgba(56, 189, 248, 0.16)'
            : 'rgba(0,0,0,0.45)'}
    />
    {#if showStunRipple}
      <ellipse cx="0" cy="18" rx="35" ry="13" fill="none" stroke={stunAccent} stroke-width="1.6" opacity="0.72" />
    {/if}
    {#if selectedNode === 'stun'}
      <ellipse cx="0" cy="16" rx="36" ry="15" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon
      points="0,-24 22,-13 0,-2 -22,-13"
      fill={!busy && effectiveVerdict === 'leaked'
        ? `url(#${prefix}-top-leak)`
        : !busy && effectiveVerdict === 'clear' && !isHaltedBranch
          ? `url(#${prefix}-top-sig)`
          : `url(#${prefix}-top)`}
      stroke={stunAccent}
      stroke-width="0.95"
      stroke-dasharray={isHaltedBranch ? '3 2' : undefined}
      stroke-opacity="0.82"
    />
    <polygon
      points="-22,-13 0,-2 0,18 -22,7"
      fill={`url(#${prefix}-left)`}
      stroke={!busy && effectiveVerdict === 'leaked' ? 'rgba(248,113,113,0.4)' : '#334155'}
      stroke-width="0.85"
    />
    <polygon
      points="0,-2 22,-13 22,7 0,18"
      fill={`url(#${prefix}-right)`}
      stroke={!busy && effectiveVerdict === 'leaked' ? 'rgba(248,113,113,0.28)' : '#1e293b'}
      stroke-width="0.85"
    />
    <line x1="0" y1="-13" x2="0" y2="-28" stroke={stunAccent} stroke-width="1.6" stroke-linecap="round" />
    <circle cx="0" cy="-29" r="2.8" fill={stunAccent} />
    <path d="M -9 -24 A 11 5 0 0 1 9 -24" fill="none" stroke={stunAccent} stroke-width="1.1" opacity="0.65" />
    <circle cx="-11" cy="3" r="2" fill={stunAccent} />
    <circle cx="-5" cy="6" r="1.5" fill="#94a3b8" opacity="0.65" />
    <circle cx="0" cy="0" r="30" fill="transparent" />
    {#if compact}
      <text
        x="0"
        y="36"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'leaked' ? '#fca5a5' : '#f8fafc'}
        font-size="11.5"
        font-weight="600"
      >
        {truncateText(stunTitle, 16)}
      </text>
      <text
        x="0"
        y="51"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'leaked' ? '#f87171' : '#94a3b8'}
        font-size="10.5"
        font-family={stunSubIsIP ? "'JetBrains Mono', ui-monospace, monospace" : undefined}
      >
        {truncateText(stunSubCompact, 18)}
      </text>
    {:else}
      <text
        x="0"
        y="-52"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'leaked' ? '#fca5a5' : '#f8fafc'}
        font-size="12"
        font-weight="600"
      >
        {truncateText(stunTitle, 28)}
      </text>
      <text
        x="0"
        y="-37"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'leaked' ? '#f87171' : '#94a3b8'}
        font-size="10.5"
        font-family={stunSubIsIP ? "'JetBrains Mono', ui-monospace, monospace" : undefined}
      >
        {truncateText(stunSub, 30)}
      </text>
    {/if}
  </g>

  {@const ex = pos.exit}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.exitAria}
    onclick={() => toggleNode('exit')}
    onkeydown={(e) => handleNodeKey(e, 'exit')}
    transform={`translate(${ex[0]}, ${ex[1]})`}
  >
    <ellipse cx="0" cy="20" rx="28" ry="10" fill={activeExit?.ip ? 'rgba(74, 222, 128, 0.14)' : 'rgba(0,0,0,0.48)'} />
    {#if showExitRipple}
      <ellipse cx="0" cy="18" rx="34" ry="13" fill="none" stroke="var(--sig)" stroke-width="1.5" opacity="0.65" />
    {/if}
    {#if selectedNode === 'exit'}
      <ellipse cx="0" cy="16" rx="35" ry="14" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon points="0,-28 21,-18 0,-8 -21,-18" fill={`url(#${prefix}-top-sig)`} stroke="var(--sig)" stroke-width="0.9" stroke-opacity={activeExit?.ip ? 0.78 : 0.38} />
    <polygon points="-21,-18 0,-8 0,18 -21,8" fill={`url(#${prefix}-left)`} stroke="#334155" stroke-width="0.85" />
    <polygon points="0,-8 21,-18 21,8 0,18" fill={`url(#${prefix}-right)`} stroke="#1e293b" stroke-width="0.85" />
    <line x1="-16" y1="-10" x2="-4" y2="-4" stroke="var(--sig)" stroke-width="1.8" stroke-linecap="round" opacity={activeExit?.ip ? 0.92 : 0.4} />
    <line x1="-16" y1="-2" x2="-4" y2="4" stroke="#38bdf8" stroke-width="1.4" stroke-linecap="round" opacity="0.55" />
    <line x1="-16" y1="6" x2="-4" y2="12" stroke="#64748b" stroke-width="1.4" stroke-linecap="round" opacity="0.55" />
    <circle cx="0" cy="0" r="30" fill="transparent" />
    {#if compact}
      <text x="0" y="36" text-anchor="middle" fill="#f8fafc" font-size="11.5" font-weight="600">{truncateText(exitName, 16)}</text>
      <text x="0" y="51" text-anchor="middle" fill="#94a3b8" font-size="10.5" font-family={activeExit?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}>{activeExit?.ip ? truncateText(exitDisplayIP, 16) : isIdle ? m.common.idle : dnsFl.exitWaitingCompact}</text>
    {:else}
      <text x="0" y="38" text-anchor="middle" fill="#f8fafc" font-size="12" font-weight="600">{truncateText(exitName, 26)}</text>
      <text x="0" y="53" text-anchor="middle" fill="#94a3b8" font-size="10.5" font-family={activeExit?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}>{activeExit?.ip ? `${truncateText(exitDisplayIP, 28)} · ${dnsFl.exitSuffix}` : isIdle ? m.common.idle : dnsFl.exitSuffix}</text>
    {/if}
  </g>

  {@const jg = pos.judge}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.judgeAria}
    onclick={() => toggleNode('judge')}
    onkeydown={(e) => handleNodeKey(e, 'judge')}
    transform={`translate(${jg[0]}, ${jg[1]})`}
  >
    <ellipse cx="0" cy="20" rx="30" ry="11" fill="rgba(0,0,0,0.52)" />
    {#if showJudgeRipple}
      {@const pulseRx = verdictPulsing ? 26 + 15 * verdictPulseT : 38}
      {@const pulseRy = verdictPulsing ? 10 + 6.5 * verdictPulseT : 16}
      <ellipse
        cx="0"
        cy="14"
        rx={pulseRx}
        ry={pulseRy}
        fill="none"
        stroke={judgeAccent}
        stroke-width="1.5"
        opacity={verdictPulsing ? 0.85 - 0.35 * verdictPulseT : 0.65}
      />
      <ellipse cx="0" cy="14" rx="28" ry="11.5" fill="none" stroke={judgeAccent} stroke-width="1.2" opacity="0.85" />
    {/if}
    {#if selectedNode === 'judge'}
      <ellipse cx="0" cy="14" rx="40" ry="17" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon points="0,-10 24,2 0,14 -24,2" fill={`url(#${prefix}-top)`} stroke={judgeAccent} stroke-width="0.9" stroke-opacity="0.7" />
    <polygon points="-24,2 0,14 0,20 -24,8" fill={`url(#${prefix}-left)`} stroke="#334155" stroke-width="0.8" />
    <polygon points="0,14 24,2 24,8 0,20" fill={`url(#${prefix}-right)`} stroke="#1e293b" stroke-width="0.8" />
    <circle cx="0" cy="-8" r="11" fill="#040d1d" stroke={judgeAccent} stroke-width="1.4" />
    <circle cx="0" cy="-8" r="4.5" fill={judgeAccent} opacity="0.95" />
    <path d="M 0 -22 V -16 M 0 0 V 6 M -14 -8 H -8 M 8 -8 H 14" stroke={judgeAccent} stroke-width="1.5" stroke-linecap="round" />
    <circle cx="0" cy="0" r="32" fill="transparent" />
    <text x="0" y={compact ? 34 : 38} text-anchor="middle" fill="#f8fafc" font-size={compact ? 11.5 : 12} font-weight="600">{fl.judgeNode}</text>
    <text x="0" y={compact ? 48 : 53} text-anchor="middle" fill={judgeAccent} font-size="10.5" font-weight="600">
      {verdictTitleText}
    </text>
  </g>
{/snippet}

{#snippet svgDefs(prefix: string)}
  <defs>
    <pattern id={`${prefix}-iso`} width="40" height="23.09" patternUnits="userSpaceOnUse">
      <path d="M 20 0 L 40 11.545 L 20 23.09 L 0 11.545 Z" fill="none" stroke="rgba(148, 163, 184, 0.08)" stroke-width="0.8" />
    </pattern>
    <linearGradient id={`${prefix}-top`} x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#334155" />
      <stop offset="100%" stop-color="#1e293b" />
    </linearGradient>
    <linearGradient id={`${prefix}-top-sig`} x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#14532d" />
      <stop offset="100%" stop-color="#0f2922" />
    </linearGradient>
    <linearGradient id={`${prefix}-top-warn`} x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#78350f" />
      <stop offset="100%" stop-color="#331c0b" />
    </linearGradient>
    <linearGradient id={`${prefix}-top-leak`} x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#7f1d1d" />
      <stop offset="100%" stop-color="#3b1116" />
    </linearGradient>
    <linearGradient id={`${prefix}-left`} x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#1e293b" />
      <stop offset="100%" stop-color="#0f172a" />
    </linearGradient>
    <linearGradient id={`${prefix}-right`} x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#0f172a" />
      <stop offset="100%" stop-color="#060c1a" />
    </linearGradient>
    <linearGradient id={`${prefix}-screen`} x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#0f2942" />
      <stop offset="100%" stop-color="#091526" />
    </linearGradient>
    <filter id={`${prefix}-glow`} x="-60%" y="-60%" width="220%" height="220%">
      <feGaussianBlur stdDeviation="3.2" result="blur" />
      <feMerge>
        <feMergeNode in="blur" />
        <feMergeNode in="SourceGraphic" />
      </feMerge>
    </filter>
  </defs>
{/snippet}

<figure class="dfs-stage" aria-label={ariaSummary}>
  <figcaption class="sr-only">{ariaSummary}</figcaption>

  {#if busy}
    <div class="dfs-status">
      <span
        aria-hidden="true"
        class="dot"
        style:background={stunAccent}
        style:--_soft="transparent"
      ></span>
      <span>{statusText}</span>
    </div>
  {/if}

  <div class="dfs-canvas-wrap">
    <svg class="dfs-svg-desktop" viewBox="0 0 760 288">
      {@render svgDefs('wd')}
      <rect width="760" height="288" fill="url(#wd-iso)" />

      <path
        d={polyToD(DESKTOP_STUN)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isHaltedBranch ? '5 4' : undefined}
      />
      <path d={polyToD(DESKTOP_HTTPS)} fill="none" stroke="rgba(148,163,184,0.16)" stroke-width="2" />

      <path
        d={polyToD(activeStunDesktopPts)}
        fill="none"
        stroke={stunAccent}
        stroke-width={busy ? 2.5 : 2.3}
        stroke-opacity={busy && liveProgress?.phase === 'gathering' && stunT < 0.22 ? 0.45 : 0.82}
        stroke-dasharray={isHaltedBranch ? '5 4' : undefined}
      />
      <path
        d={polyToD(DESKTOP_HTTPS)}
        fill="none"
        stroke="var(--sig)"
        stroke-width={activeExit?.ip ? 2.4 : 1.8}
        stroke-opacity={activeExit?.ip ? 0.78 : 0.45}
      />

      <g transform="translate(252, 106)" opacity="0.88">
        <rect x={-wireStunW / 2} y="-9.5" width={wireStunW} height="19" rx="9.5" fill="#050d1d" stroke={stunAccent} stroke-opacity="0.4" />
        <text x="0" y="3.2" text-anchor="middle" fill="#cbd5e1" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">{wireStunLabel}</text>
      </g>
      <g transform="translate(508, 106)" opacity="0.88">
        <rect x={-wireReadW / 2} y="-9.5" width={wireReadW} height="19" rx="9.5" fill="#050d1d" stroke="rgba(148,163,184,0.28)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#cbd5e1" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">
          {wireReadLabel}
        </text>
      </g>
      <g transform="translate(252, 190)" opacity="0.88">
        <rect x="-24" y="-9.5" width="48" height="19" rx="9.5" fill="#050d1d" stroke="rgba(74,222,128,0.38)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#86efac" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">{fl.wireHttps}</text>
      </g>
      <g transform="translate(508, 190)" opacity="0.88">
        <rect x="-19" y="-9.5" width="38" height="19" rx="9.5" fill="#050d1d" stroke="rgba(74,222,128,0.28)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#86efac" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">{fl.wireApi}</text>
      </g>

      {#if isHaltedBranch}
        <text x="324" y="88" text-anchor="middle" fill="var(--unknown)" font-size="14" font-weight="700">×</text>
      {/if}

      {@render isoNodes('wd', NODE_POS_DESKTOP, false)}

      {#if httpsActive && httpsT > 0.05}
        {#if httpsT > 0.09 && httpsT < 0.98}
          <circle cx={httpsTrail2Desktop.x} cy={httpsTrail2Desktop.y} r="2.2" fill="var(--sig)" opacity="0.24" />
          <circle cx={httpsTrail1Desktop.x} cy={httpsTrail1Desktop.y} r="3.2" fill="var(--sig)" opacity="0.46" />
        {/if}
        <g transform={`translate(${httpsPosDesktop.x}, ${httpsPosDesktop.y})`} filter="url(#wd-glow)">
          <rect x="-26" y="-11" width="52" height="22" rx="11" fill="var(--sig)" opacity="0.22" />
          <rect x="-24" y="-9" width="48" height="18" rx="9" fill="#071122" stroke="var(--sig)" stroke-width="1.45" />
          <text x="0" y="3.3" text-anchor="middle" fill="#f8fafc" font-size="9.5" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">HTTPS</text>
        </g>
      {/if}

      {#if stunActive && stunT > 0.05}
        {#if stunT > 0.09 && stunT < 0.98}
          <circle cx={stunTrail2Desktop.x} cy={stunTrail2Desktop.y} r="2.2" fill={stunAccent} opacity="0.24" />
          <circle cx={stunTrail1Desktop.x} cy={stunTrail1Desktop.y} r="3.2" fill={stunAccent} opacity="0.46" />
        {/if}
        <g transform={`translate(${stunPosDesktop.x}, ${stunPosDesktop.y})`} filter="url(#wd-glow)">
          <rect x="-29" y="-11" width="58" height="22" rx="11" fill={stunAccent} opacity="0.22" />
          <rect x="-27" y="-9" width="54" height="18" rx="9" fill="#071122" stroke={stunAccent} stroke-width="1.45" />
          <text x="0" y="3.3" text-anchor="middle" fill="#f8fafc" font-size="9.5" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">WebRTC</text>
        </g>
      {/if}
    </svg>

    <svg class="dfs-svg-mobile" viewBox="0 0 340 314">
      {@render svgDefs('wm')}
      <rect width="340" height="314" fill="url(#wm-iso)" />

      <path
        d={polyToD(MOBILE_STUN)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isHaltedBranch ? '5 4' : undefined}
      />
      <path d={polyToD(MOBILE_HTTPS)} fill="none" stroke="rgba(148,163,184,0.16)" stroke-width="2" />

      <path
        d={polyToD(activeStunMobilePts)}
        fill="none"
        stroke={stunAccent}
        stroke-width={busy ? 2.5 : 2.3}
        stroke-opacity={busy && liveProgress?.phase === 'gathering' && stunT < 0.22 ? 0.45 : 0.82}
        stroke-dasharray={isHaltedBranch ? '5 4' : undefined}
      />
      <path
        d={polyToD(MOBILE_HTTPS)}
        fill="none"
        stroke="var(--sig)"
        stroke-width={activeExit?.ip ? 2.4 : 1.8}
        stroke-opacity={activeExit?.ip ? 0.78 : 0.45}
      />

      <g transform="translate(114, 102)" opacity="0.88">
        <rect x="-25" y="-8.5" width="50" height="17" rx="8.5" fill="#050d1d" stroke={stunAccent} stroke-opacity="0.4" />
        <text x="0" y="3" text-anchor="middle" fill="#cbd5e1" font-size="9" font-family="'JetBrains Mono', ui-monospace, monospace">WebRTC</text>
      </g>
      <g transform="translate(226, 102)" opacity="0.88">
        <rect x="-23" y="-8.5" width="46" height="17" rx="8.5" fill="#050d1d" stroke="rgba(74,222,128,0.38)" />
        <text x="0" y="3" text-anchor="middle" fill="#86efac" font-size="9" font-family="'JetBrains Mono', ui-monospace, monospace">HTTPS</text>
      </g>

      {#if isHaltedBranch}
        <text x="78" y="106" text-anchor="middle" fill="var(--unknown)" font-size="13" font-weight="700">×</text>
      {/if}

      {@render isoNodes('wm', NODE_POS_MOBILE, true)}

      {#if httpsActive && httpsT > 0.05}
        {#if httpsT > 0.09 && httpsT < 0.98}
          <circle cx={httpsTrail2Mobile.x} cy={httpsTrail2Mobile.y} r="2" fill="var(--sig)" opacity="0.24" />
          <circle cx={httpsTrail1Mobile.x} cy={httpsTrail1Mobile.y} r="3" fill="var(--sig)" opacity="0.46" />
        {/if}
        <g transform={`translate(${httpsPosMobile.x}, ${httpsPosMobile.y})`} filter="url(#wm-glow)">
          <rect x="-24" y="-10" width="48" height="20" rx="10" fill="var(--sig)" opacity="0.24" />
          <rect x="-22" y="-8" width="44" height="16" rx="8" fill="#071122" stroke="var(--sig)" stroke-width="1.4" />
          <text x="0" y="3" text-anchor="middle" fill="#f8fafc" font-size="8.8" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">HTTPS</text>
        </g>
      {/if}

      {#if stunActive && stunT > 0.05}
        {#if stunT > 0.09 && stunT < 0.98}
          <circle cx={stunTrail2Mobile.x} cy={stunTrail2Mobile.y} r="2" fill={stunAccent} opacity="0.24" />
          <circle cx={stunTrail1Mobile.x} cy={stunTrail1Mobile.y} r="3" fill={stunAccent} opacity="0.46" />
        {/if}
        <g transform={`translate(${stunPosMobile.x}, ${stunPosMobile.y})`} filter="url(#wm-glow)">
          <rect x="-26" y="-10" width="52" height="20" rx="10" fill={stunAccent} opacity="0.24" />
          <rect x="-24" y="-8" width="48" height="16" rx="8" fill="#071122" stroke={stunAccent} stroke-width="1.4" />
          <text x="0" y="3" text-anchor="middle" fill="#f8fafc" font-size="8.8" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">WebRTC</text>
        </g>
      {/if}
    </svg>

    {#if inspectorInfo}
      <div class="dfs-inspector" role="status">
        <span>{inspectorInfo}</span>
        <button
          type="button"
          class="text-(--slate-400) hover:text-white px-1 cursor-pointer"
          aria-label={m.home.feedback.close}
          onclick={() => {
            selectedNode = null;
          }}
        >
          ×
        </button>
      </div>
    {/if}
  </div>
</figure>
