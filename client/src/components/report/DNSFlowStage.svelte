<script lang="ts">
  import { onDestroy } from 'svelte';
  import { getMessages, joinList, type Locale } from '../../i18n';
  import type { DNSProgress } from '../../lib/dnsCheck';
  import { dnsProviderName, dnsRegion, isLiveResolverSuspect } from '../../lib/dnsPresentation';
  import { maskIP } from '../../lib/ipDisplay';
  import type { DNSExit } from '../../lib/types';

  export interface DNSFlowProviderSummary {
    name: string;
    country: string;
    type?: 'public' | 'access' | 'hosting';
    suspect: boolean;
    count: number;
  }

  type VerdictState = 'suspected' | 'clear' | 'unknown' | 'unobserved';
  type NodeId = 'browser' | 'resolver' | 'exit' | 'authority';
  type Pt = readonly [number, number];

  interface Props {
    locale?: Locale;
    exit: DNSExit | null;
    verdictState: VerdictState;
    providers: DNSFlowProviderSummary[];
    resolverCount: number;
    revealExit: boolean;
    busy?: boolean;
    liveProgress?: DNSProgress | null;
    hasResult?: boolean;
  }

  let {
    locale = 'zh-CN',
    exit,
    verdictState,
    providers,
    resolverCount,
    revealExit,
    busy = false,
    liveProgress = null,
    hasResult = false,
  }: Props = $props();

  let m = $derived(getMessages(locale));
  let fl = $derived(m.dns.flow);

  let httpsT = $state(1);
  let httpsActive = $state(false);
  let dnsT = $state(1);
  let dnsActive = $state(false);
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

  let dnsPhase: 'idle' | 'probing' | 'collecting' | 'assessing' | 'finishing' | 'docked' = 'idle';
  let dnsPhaseStartAt = 0;
  let dnsFromT = 0;

  let pendingVerdictRipple = false;
  let verdictRippleStartAt = 0;

  const DESKTOP_DNS: readonly Pt[] = [
    [106, 148],
    [186, 84],
    [380, 84],
    [574, 84],
    [654, 148],
  ];
  const DESKTOP_DNS_HALT: readonly Pt[] = [
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

  const MOBILE_DNS: readonly Pt[] = [
    [170, 42],
    [78, 42],
    [78, 152],
    [166, 152],
    [166, 254],
  ];
  const MOBILE_DNS_HALT: readonly Pt[] = [
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
    resolver: [380, 84],
    exit: [380, 212],
    authority: [654, 148],
  };

  const NODE_POS_MOBILE: Record<NodeId, Pt> = {
    browser: [170, 42],
    resolver: [78, 152],
    exit: [262, 152],
    authority: [170, 254],
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
      const u = Math.min(1, (now - httpsArrivedAt) / 1400);
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

    if (dnsPhase === 'probing') {
      dnsActive = true;
      const dur = 2400;
      const cycle = ((now - scanStartAt) % dur) / dur;
      if (cycle < 0.72) {
        dnsT = easeInOut(cycle / 0.72) * 0.51;
      } else {
        dnsT = 0.51 - Math.sin(((cycle - 0.72) / 0.28) * Math.PI) * 0.025;
      }
      keepGoing = true;
    } else if (dnsPhase === 'collecting') {
      const u = Math.min(1, (now - dnsPhaseStartAt) / 1400);
      dnsT = dnsFromT + easeInOut(u) * (1 - dnsFromT);
      if (u >= 1) {
        dnsT = 1;
        dnsActive = false;
        dnsPhase = 'docked';
      } else {
        dnsActive = true;
        keepGoing = true;
      }
    } else if (dnsPhase === 'finishing') {
      const u = Math.min(1, (now - dnsPhaseStartAt) / 750);
      dnsT = dnsFromT + easeInOut(u) * (1 - dnsFromT);
      if (u >= 1) {
        dnsT = 1;
        dnsActive = false;
        dnsPhase = 'docked';
      } else {
        dnsActive = true;
        keepGoing = true;
      }
    }

    if (pendingVerdictRipple && httpsPhase === 'docked' && dnsPhase === 'docked') {
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

  let activeProviders = $derived.by<DNSFlowProviderSummary[]>(() => {
    if (busy && liveProgress) {
      return liveProgress.resolvers.map((r) => ({
        name: dnsProviderName(r.isp, locale) || r.isp || fl.resolverDefault,
        country: r.country,
        type: r.network_type,
        suspect: isLiveResolverSuspect(activeExit, r),
        count: 1,
      }));
    }
    return providers;
  });

  let activeResolverCount = $derived(
    busy && liveProgress ? liveProgress.resolvers.length : resolverCount,
  );

  let effectiveVerdict = $derived<VerdictState>(busy ? 'unknown' : verdictState);
  let isUnobservedBranch = $derived(!busy && effectiveVerdict === 'unobserved');

  $effect(() => {
    const reduced = prefersReducedMotion();
    const hasLiveExit = Boolean(activeExit?.ip);
    const livePhase = liveProgress?.phase ?? 'probing';

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
          dnsPhase = livePhase;
          dnsT = livePhase === 'probing' ? 0.5 : 1;
          dnsActive = false;
          stopAnim();
          return;
        }

        httpsPhase = hasLiveExit ? 'arriving' : 'probing';
        httpsArrivedAt = scanStartAt;
        httpsFromT = 0;
        httpsT = 0;
        httpsActive = true;

        dnsPhase = livePhase === 'assessing' ? 'docked' : livePhase;
        dnsPhaseStartAt = scanStartAt;
        dnsFromT = 0;
        dnsT = 0;
        dnsActive = dnsPhase !== 'docked';
        ensureRaf();
      } else if (reduced) {
        if (hasLiveExit) {
          httpsPhase = 'docked';
          httpsT = 1;
        }
        dnsT = livePhase === 'probing' ? 0.5 : 1;
      } else {
        if (hasLiveExit && httpsPhase === 'probing') {
          httpsPhase = 'arriving';
          httpsArrivedAt = performance.now();
          httpsFromT = httpsT;
          ensureRaf();
        }
        if (livePhase === 'collecting' && dnsPhase === 'probing') {
          dnsPhase = 'collecting';
          dnsPhaseStartAt = performance.now();
          dnsFromT = Math.max(dnsT, 0.48);
          dnsActive = true;
          ensureRaf();
        } else if (livePhase === 'assessing') {
          if (dnsPhase === 'probing') {
            dnsPhase = 'collecting';
            dnsPhaseStartAt = performance.now();
            dnsFromT = Math.max(dnsT, 0.48);
            dnsActive = true;
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
        dnsPhase = 'docked';
        dnsT = 1;
        dnsActive = false;
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
        if (dnsActive && !isUnobservedBranch && dnsT < 0.98) {
          dnsPhase = 'finishing';
          dnsPhaseStartAt = now;
          dnsFromT = dnsT;
        } else {
          dnsPhase = 'docked';
          dnsT = 1;
          dnsActive = false;
        }
        pendingVerdictRipple = true;
        ensureRaf();
      }
    } else if (!hasResult || (!pendingVerdictRipple && !verdictPulsing && httpsPhase !== 'finishing' && dnsPhase !== 'finishing')) {
      hadLiveScan = false;
      pendingVerdictRipple = false;
      httpsPhase = 'docked';
      httpsT = 1;
      httpsActive = false;
      dnsPhase = 'docked';
      dnsT = 1;
      dnsActive = false;
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

  let isIdle = $derived(!busy && !hasResult);

  let exitName = $derived(
    activeExit?.ip
      ? `${dnsProviderName(activeExit.network, locale) || fl.exitDefault} · ${dnsRegion(activeExit.country, locale)}`
      : isIdle
        ? fl.exitIdle
        : fl.exitFetching,
  );
  let exitDisplayIP = $derived(
    activeExit?.ip ? (revealExit ? activeExit.ip : maskIP(activeExit.ip)) : isIdle ? m.common.idle : '…',
  );

  let providerSummaryText = $derived.by(() => {
    const list =
      effectiveVerdict === 'suspected'
        ? activeProviders.filter((p) => p.suspect)
        : activeProviders;
    const picked = (list.length ? list : activeProviders)
      .map((p) => p.name)
      .filter(Boolean);
    const unique = [...new Set(picked)];
    if (!unique.length) return fl.resolverDefault;
    const joined = joinList(unique.slice(0, 2), locale);
    return unique.length > 2 ? fl.moreProviders(joined) : joined;
  });

  let resolverTitle = $derived.by(() => {
    if (isIdle) return fl.resolverIdle;
    if (busy && activeResolverCount === 0) return fl.resolverWaiting;
    if (!busy && effectiveVerdict === 'unobserved') return fl.resolverUnobserved;
    if (!busy && effectiveVerdict === 'unknown') {
      return activeProviders.length ? providerSummaryText : fl.resolverUnknown;
    }
    const regionCode =
      effectiveVerdict === 'suspected'
        ? activeProviders.find((p) => p.suspect)?.country ?? activeProviders[0]?.country
        : activeProviders.length === 1 || busy
          ? activeProviders[0]?.country
          : '';
    return regionCode ? `${providerSummaryText} · ${dnsRegion(regionCode, locale)}` : providerSummaryText;
  });

  let resolverSub = $derived.by(() => {
    if (isIdle) return m.common.idle;
    if (busy) {
      if (liveProgress?.phase === 'collecting') {
        return fl.subCollecting(liveProgress.completedRounds, liveProgress.totalRounds, activeResolverCount);
      }
      if (liveProgress?.phase === 'assessing') {
        return fl.subAssessing(activeResolverCount);
      }
      const started = Math.max(1, liveProgress?.startedRounds ?? 1);
      const total = liveProgress?.totalRounds ?? 3;
      return fl.subProbing(started, total);
    }
    if (effectiveVerdict === 'suspected') return fl.subSuspected;
    if (effectiveVerdict === 'clear') {
      if (activeProviders.length > 0 && activeProviders.every((p) => p.type === 'public')) {
        return fl.subClearPublic(activeResolverCount);
      }
      return fl.subClearCount(activeResolverCount);
    }
    if (effectiveVerdict === 'unobserved') return fl.subUnobserved;
    return fl.subUnknownCount(activeResolverCount);
  });

  let dnsAccent = $derived(
    busy
      ? liveProgress?.phase === 'collecting' || liveProgress?.phase === 'assessing'
        ? '#38bdf8'
        : 'var(--warn)'
      : effectiveVerdict === 'suspected'
        ? 'var(--leak)'
        : effectiveVerdict === 'clear'
          ? 'var(--sig)'
          : 'var(--unknown)',
  );

  let activeDnsDesktopPts = $derived(isUnobservedBranch ? DESKTOP_DNS_HALT : DESKTOP_DNS);
  let activeDnsMobilePts = $derived(isUnobservedBranch ? MOBILE_DNS_HALT : MOBILE_DNS);

  let dnsPosDesktop = $derived(pointAlong(activeDnsDesktopPts, dnsT));
  let dnsTrail1Desktop = $derived(pointAlong(activeDnsDesktopPts, dnsT - 0.028));
  let dnsTrail2Desktop = $derived(pointAlong(activeDnsDesktopPts, dnsT - 0.054));

  let httpsPosDesktop = $derived(pointAlong(DESKTOP_HTTPS, httpsT));
  let httpsTrail1Desktop = $derived(pointAlong(DESKTOP_HTTPS, httpsT - 0.028));
  let httpsTrail2Desktop = $derived(pointAlong(DESKTOP_HTTPS, httpsT - 0.054));

  let dnsPosMobile = $derived(pointAlong(activeDnsMobilePts, dnsT));
  let dnsTrail1Mobile = $derived(pointAlong(activeDnsMobilePts, dnsT - 0.028));
  let dnsTrail2Mobile = $derived(pointAlong(activeDnsMobilePts, dnsT - 0.054));

  let httpsPosMobile = $derived(pointAlong(MOBILE_HTTPS, httpsT));
  let httpsTrail1Mobile = $derived(pointAlong(MOBILE_HTTPS, httpsT - 0.028));
  let httpsTrail2Mobile = $derived(pointAlong(MOBILE_HTTPS, httpsT - 0.054));

  let showResolverRipple = $derived(
    busy && (liveProgress?.phase === 'collecting' || (!isUnobservedBranch && dnsT >= 0.44)),
  );
  let showExitRipple = $derived(
    busy && Boolean(activeExit?.ip) && httpsT >= 0.44,
  );
  let showAuthorityRipple = $derived(
    (!busy && hasResult && !pendingVerdictRipple) ||
    (busy && (liveProgress?.phase === 'assessing' || (liveProgress?.phase === 'collecting' && dnsT > 0.94))),
  );

  let verdictTitleText = $derived(
    isIdle
      ? m.common.idle
      : busy
        ? liveProgress?.phase === 'assessing'
          ? fl.verdictAssessing
          : fl.verdictSampling
        : m.dns.status[effectiveVerdict],
  );

  let inspectorInfo = $derived.by(() => {
    if (!selectedNode) return null;
    switch (selectedNode) {
      case 'browser':
        return fl.inspectorBrowser;
      case 'resolver':
        return fl.inspectorResolver(resolverTitle, resolverSub);
      case 'exit':
        return fl.inspectorExit(exitName, exitDisplayIP, activeExit?.asn);
      case 'authority':
        return fl.inspectorAuthority;
    }
  });

  let statusText = $derived.by(() => {
    if (!busy) return '';
    if (!liveProgress || liveProgress.phase === 'probing') {
      const started = Math.max(1, liveProgress?.startedRounds ?? 1);
      const total = liveProgress?.totalRounds ?? 3;
      return fl.statusProbing(started, total);
    }
    if (liveProgress.phase === 'collecting') {
      return fl.statusCollecting(liveProgress.completedRounds, liveProgress.totalRounds, liveProgress.resolvers.length);
    }
    return fl.statusAssessing;
  });

  let ariaSummary = $derived(
    isIdle
      ? fl.ariaIdle
      : busy && !hasResult
        ? fl.ariaBusy
        : fl.ariaResult(resolverTitle, resolverSub, exitName, exitDisplayIP, verdictTitleText),
  );

  let wireDnsLabel = $derived(fl.wireDns);
  let wireAuthLabel = $derived(isUnobservedBranch ? fl.wireNoCapture : fl.wireAuth);
  let wireDnsW = $derived(pillWidth(wireDnsLabel, 44));
  let wireAuthW = $derived(pillWidth(wireAuthLabel, 48));
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

  {@const res = pos.resolver}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.resolverAria}
    onclick={() => toggleNode('resolver')}
    onkeydown={(e) => handleNodeKey(e, 'resolver')}
    transform={`translate(${res[0]}, ${res[1]})`}
  >
    <ellipse
      cx="0"
      cy="20"
      rx="30"
      ry="11"
      fill={!busy && effectiveVerdict === 'suspected'
        ? 'rgba(248, 113, 113, 0.18)'
        : !busy && effectiveVerdict === 'clear'
          ? 'rgba(74, 222, 128, 0.14)'
          : busy && activeResolverCount > 0
            ? 'rgba(56, 189, 248, 0.16)'
            : 'rgba(0,0,0,0.45)'}
    />
    {#if showResolverRipple}
      <ellipse cx="0" cy="18" rx="35" ry="13" fill="none" stroke={dnsAccent} stroke-width="1.6" opacity="0.72" />
    {/if}
    {#if selectedNode === 'resolver'}
      <ellipse cx="0" cy="16" rx="36" ry="15" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon
      points="0,-24 22,-13 0,-2 -22,-13"
      fill={!busy && effectiveVerdict === 'suspected'
        ? `url(#${prefix}-top-leak)`
        : !busy && effectiveVerdict === 'clear'
          ? `url(#${prefix}-top-sig)`
          : `url(#${prefix}-top)`}
      stroke={dnsAccent}
      stroke-width="0.95"
      stroke-dasharray={isUnobservedBranch ? '3 2' : undefined}
      stroke-opacity="0.82"
    />
    <polygon
      points="-22,-13 0,-2 0,18 -22,7"
      fill={`url(#${prefix}-left)`}
      stroke={!busy && effectiveVerdict === 'suspected' ? 'rgba(248,113,113,0.4)' : '#334155'}
      stroke-width="0.85"
    />
    <polygon
      points="0,-2 22,-13 22,7 0,18"
      fill={`url(#${prefix}-right)`}
      stroke={!busy && effectiveVerdict === 'suspected' ? 'rgba(248,113,113,0.28)' : '#1e293b'}
      stroke-width="0.85"
    />
    <line x1="0" y1="-13" x2="0" y2="-28" stroke={dnsAccent} stroke-width="1.6" stroke-linecap="round" />
    <circle cx="0" cy="-29" r="2.8" fill={dnsAccent} />
    <path d="M -9 -24 A 11 5 0 0 1 9 -24" fill="none" stroke={dnsAccent} stroke-width="1.1" opacity="0.65" />
    <circle cx="-11" cy="3" r="2" fill={dnsAccent} />
    <circle cx="-5" cy="6" r="1.5" fill="#94a3b8" opacity="0.65" />
    <circle cx="0" cy="0" r="30" fill="transparent" />
    {#if compact}
      <text
        x="0"
        y="36"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'suspected' ? '#fca5a5' : '#f8fafc'}
        font-size="11.5"
        font-weight="600"
      >
        {truncateText(resolverTitle, 16)}
      </text>
      <text
        x="0"
        y="51"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'suspected' ? '#f87171' : '#94a3b8'}
        font-size="10.5"
      >
        {truncateText(resolverSub, 18)}
      </text>
    {:else}
      <text
        x="0"
        y="-52"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'suspected' ? '#fca5a5' : '#f8fafc'}
        font-size="12"
        font-weight="600"
      >
        {truncateText(resolverTitle, 28)}
      </text>
      <text
        x="0"
        y="-37"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'suspected' ? '#f87171' : '#94a3b8'}
        font-size="10.5"
      >
        {truncateText(resolverSub, 30)}
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
      <text x="0" y="51" text-anchor="middle" fill="#94a3b8" font-size="10.5" font-family={activeExit?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}>{activeExit?.ip ? exitDisplayIP : isIdle ? m.common.idle : fl.exitWaitingCompact}</text>
    {:else}
      <text x="0" y="38" text-anchor="middle" fill="#f8fafc" font-size="12" font-weight="600">{truncateText(exitName, 26)}</text>
      <text x="0" y="53" text-anchor="middle" fill="#94a3b8" font-size="10.5" font-family={activeExit?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}>{activeExit?.ip ? `${exitDisplayIP} · ${fl.exitSuffix}` : isIdle ? m.common.idle : fl.exitSuffix}</text>
    {/if}
  </g>

  {@const au = pos.authority}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.authorityAria}
    onclick={() => toggleNode('authority')}
    onkeydown={(e) => handleNodeKey(e, 'authority')}
    transform={`translate(${au[0]}, ${au[1]})`}
  >
    <ellipse cx="0" cy="20" rx="30" ry="11" fill="rgba(0,0,0,0.52)" />
    {#if showAuthorityRipple}
      {@const pulseRx = verdictPulsing ? 26 + 15 * verdictPulseT : 38}
      {@const pulseRy = verdictPulsing ? 10 + 6.5 * verdictPulseT : 16}
      <ellipse
        cx="0"
        cy="14"
        rx={pulseRx}
        ry={pulseRy}
        fill="none"
        stroke={dnsAccent}
        stroke-width="1.5"
        opacity={verdictPulsing ? 0.85 - 0.35 * verdictPulseT : 0.65}
      />
      <ellipse cx="0" cy="14" rx="28" ry="11.5" fill="none" stroke={dnsAccent} stroke-width="1.2" opacity="0.85" />
    {/if}
    {#if selectedNode === 'authority'}
      <ellipse cx="0" cy="14" rx="40" ry="17" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon points="0,-10 24,2 0,14 -24,2" fill={`url(#${prefix}-top)`} stroke={dnsAccent} stroke-width="0.9" stroke-opacity="0.7" />
    <polygon points="-24,2 0,14 0,20 -24,8" fill={`url(#${prefix}-left)`} stroke="#334155" stroke-width="0.8" />
    <polygon points="0,14 24,2 24,8 0,20" fill={`url(#${prefix}-right)`} stroke="#1e293b" stroke-width="0.8" />
    <circle cx="0" cy="-8" r="11" fill="#040d1d" stroke={dnsAccent} stroke-width="1.4" />
    <circle cx="0" cy="-8" r="4.5" fill={dnsAccent} opacity="0.95" />
    <path d="M 0 -22 V -16 M 0 0 V 6 M -14 -8 H -8 M 8 -8 H 14" stroke={dnsAccent} stroke-width="1.5" stroke-linecap="round" />
    <circle cx="0" cy="0" r="32" fill="transparent" />
    <text x="0" y={compact ? 34 : 38} text-anchor="middle" fill="#f8fafc" font-size={compact ? 11.5 : 12} font-weight="600">{fl.authorityNode}</text>
    <text x="0" y={compact ? 48 : 53} text-anchor="middle" fill={dnsAccent} font-size="10.5" font-weight="600">
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
        style:background={dnsAccent}
        style:--_soft="transparent"
      ></span>
      <span>{statusText}</span>
    </div>
  {/if}

  <div class="dfs-canvas-wrap">
    <svg class="dfs-svg-desktop" viewBox="0 0 760 288">
      {@render svgDefs('d')}
      <rect width="760" height="288" fill="url(#d-iso)" />

      <path
        d={polyToD(DESKTOP_DNS)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isUnobservedBranch ? '5 4' : undefined}
      />
      <path d={polyToD(DESKTOP_HTTPS)} fill="none" stroke="rgba(148,163,184,0.16)" stroke-width="2" />

      <path
        d={polyToD(activeDnsDesktopPts)}
        fill="none"
        stroke={dnsAccent}
        stroke-width={busy ? 2.5 : 2.3}
        stroke-opacity={busy && liveProgress?.phase === 'probing' && dnsT < 0.22 ? 0.45 : 0.82}
        stroke-dasharray={isUnobservedBranch ? '5 4' : undefined}
      />

      <path
        d={polyToD(DESKTOP_HTTPS)}
        fill="none"
        stroke="var(--sig)"
        stroke-width={activeExit?.ip ? 2.4 : 1.8}
        stroke-opacity={activeExit?.ip ? 0.78 : 0.45}
      />

      <g transform="translate(252, 106)" opacity="0.88">
        <rect x={-wireDnsW / 2} y="-9.5" width={wireDnsW} height="19" rx="9.5" fill="#050d1d" stroke={dnsAccent} stroke-opacity="0.4" />
        <text x="0" y="3.2" text-anchor="middle" fill="#cbd5e1" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">{wireDnsLabel}</text>
      </g>
      <g transform="translate(508, 106)" opacity="0.88">
        <rect x={-wireAuthW / 2} y="-9.5" width={wireAuthW} height="19" rx="9.5" fill="#050d1d" stroke="rgba(148,163,184,0.28)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#cbd5e1" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">
          {wireAuthLabel}
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

      {#if isUnobservedBranch}
        <text x="324" y="88" text-anchor="middle" fill="var(--unknown)" font-size="14" font-weight="700">×</text>
      {/if}

      {@render isoNodes('d', NODE_POS_DESKTOP, false)}

      {#if httpsActive && httpsT > 0.05}
        {#if httpsT > 0.09 && httpsT < 0.98}
          <circle cx={httpsTrail2Desktop.x} cy={httpsTrail2Desktop.y} r="2.2" fill="var(--sig)" opacity="0.24" />
          <circle cx={httpsTrail1Desktop.x} cy={httpsTrail1Desktop.y} r="3.2" fill="var(--sig)" opacity="0.46" />
        {/if}
        <g transform={`translate(${httpsPosDesktop.x}, ${httpsPosDesktop.y})`} filter="url(#d-glow)">
          <rect x="-26" y="-11" width="52" height="22" rx="11" fill="var(--sig)" opacity="0.22" />
          <rect x="-24" y="-9" width="48" height="18" rx="9" fill="#071122" stroke="var(--sig)" stroke-width="1.45" />
          <text x="0" y="3.3" text-anchor="middle" fill="#f8fafc" font-size="9.5" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">HTTPS</text>
        </g>
      {/if}

      {#if dnsActive && dnsT > 0.05}
        {#if dnsT > 0.09 && dnsT < 0.98}
          <circle cx={dnsTrail2Desktop.x} cy={dnsTrail2Desktop.y} r="2.2" fill={dnsAccent} opacity="0.24" />
          <circle cx={dnsTrail1Desktop.x} cy={dnsTrail1Desktop.y} r="3.2" fill={dnsAccent} opacity="0.46" />
        {/if}
        <g transform={`translate(${dnsPosDesktop.x}, ${dnsPosDesktop.y})`} filter="url(#d-glow)">
          <rect x="-23" y="-11" width="46" height="22" rx="11" fill={dnsAccent} opacity="0.22" />
          <rect x="-21" y="-9" width="42" height="18" rx="9" fill="#071122" stroke={dnsAccent} stroke-width="1.45" />
          <text x="0" y="3.3" text-anchor="middle" fill="#f8fafc" font-size="9.5" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">DNS</text>
        </g>
      {/if}
    </svg>

    <svg class="dfs-svg-mobile" viewBox="0 0 340 314">
      {@render svgDefs('m')}
      <rect width="340" height="314" fill="url(#m-iso)" />

      <path
        d={polyToD(MOBILE_DNS)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isUnobservedBranch ? '5 4' : undefined}
      />
      <path d={polyToD(MOBILE_HTTPS)} fill="none" stroke="rgba(148,163,184,0.16)" stroke-width="2" />

      <path
        d={polyToD(activeDnsMobilePts)}
        fill="none"
        stroke={dnsAccent}
        stroke-width={busy ? 2.5 : 2.3}
        stroke-opacity={busy && liveProgress?.phase === 'probing' && dnsT < 0.22 ? 0.45 : 0.82}
        stroke-dasharray={isUnobservedBranch ? '5 4' : undefined}
      />
      <path
        d={polyToD(MOBILE_HTTPS)}
        fill="none"
        stroke="var(--sig)"
        stroke-width={activeExit?.ip ? 2.4 : 1.8}
        stroke-opacity={activeExit?.ip ? 0.78 : 0.45}
      />

      <g transform="translate(114, 102)" opacity="0.88">
        <rect x="-22" y="-8.5" width="44" height="17" rx="8.5" fill="#050d1d" stroke={dnsAccent} stroke-opacity="0.4" />
        <text x="0" y="3" text-anchor="middle" fill="#cbd5e1" font-size="9" font-family="'JetBrains Mono', ui-monospace, monospace">DNS</text>
      </g>
      <g transform="translate(226, 102)" opacity="0.88">
        <rect x="-23" y="-8.5" width="46" height="17" rx="8.5" fill="#050d1d" stroke="rgba(74,222,128,0.38)" />
        <text x="0" y="3" text-anchor="middle" fill="#86efac" font-size="9" font-family="'JetBrains Mono', ui-monospace, monospace">HTTPS</text>
      </g>

      {#if isUnobservedBranch}
        <text x="78" y="106" text-anchor="middle" fill="var(--unknown)" font-size="13" font-weight="700">×</text>
      {/if}

      {@render isoNodes('m', NODE_POS_MOBILE, true)}

      {#if httpsActive && httpsT > 0.05}
        {#if httpsT > 0.09 && httpsT < 0.98}
          <circle cx={httpsTrail2Mobile.x} cy={httpsTrail2Mobile.y} r="2" fill="var(--sig)" opacity="0.24" />
          <circle cx={httpsTrail1Mobile.x} cy={httpsTrail1Mobile.y} r="3" fill="var(--sig)" opacity="0.46" />
        {/if}
        <g transform={`translate(${httpsPosMobile.x}, ${httpsPosMobile.y})`} filter="url(#m-glow)">
          <rect x="-24" y="-10" width="48" height="20" rx="10" fill="var(--sig)" opacity="0.24" />
          <rect x="-22" y="-8" width="44" height="16" rx="8" fill="#071122" stroke="var(--sig)" stroke-width="1.4" />
          <text x="0" y="3" text-anchor="middle" fill="#f8fafc" font-size="8.8" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">HTTPS</text>
        </g>
      {/if}

      {#if dnsActive && dnsT > 0.05}
        {#if dnsT > 0.09 && dnsT < 0.98}
          <circle cx={dnsTrail2Mobile.x} cy={dnsTrail2Mobile.y} r="2" fill={dnsAccent} opacity="0.24" />
          <circle cx={dnsTrail1Mobile.x} cy={dnsTrail1Mobile.y} r="3" fill={dnsAccent} opacity="0.46" />
        {/if}
        <g transform={`translate(${dnsPosMobile.x}, ${dnsPosMobile.y})`} filter="url(#m-glow)">
          <rect x="-21" y="-10" width="42" height="20" rx="10" fill={dnsAccent} opacity="0.24" />
          <rect x="-19" y="-8" width="38" height="16" rx="8" fill="#071122" stroke={dnsAccent} stroke-width="1.4" />
          <text x="0" y="3" text-anchor="middle" fill="#f8fafc" font-size="8.8" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">DNS</text>
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
