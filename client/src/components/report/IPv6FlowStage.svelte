<script lang="ts">
  import { onDestroy } from 'svelte';
  import { getMessages, type Locale } from '../../i18n';
  import { dnsProvider } from '../../lib/dnsPresentation';
  import { maskIP } from '../../lib/ipDisplay';
  import {
    stackRegionLabel,
    type IPv6Progress,
    type IPv6VerdictState,
    type StackExit,
  } from '../../lib/ipv6Check';

  type NodeId = 'browser' | 'v6' | 'v4' | 'judge';
  type Pt = readonly [number, number];

  interface Props {
    locale?: Locale;
    v4: StackExit | null;
    v6: StackExit | null;
    verdictState: IPv6VerdictState;
    verdictNote?: string;
    revealExit: boolean;
    busy?: boolean;
    liveProgress?: IPv6Progress | null;
    hasResult?: boolean;
  }

  let {
    locale = 'zh-CN',
    v4,
    v6,
    verdictState,
    verdictNote = '',
    revealExit,
    busy = false,
    liveProgress = null,
    hasResult = false,
  }: Props = $props();

  let m = $derived(getMessages(locale));
  let fl = $derived(m.ipv6.flow);

  let v4T = $state(1);
  let v4Active = $state(false);
  let v6T = $state(1);
  let v6Active = $state(false);
  let verdictPulseT = $state(1);
  let verdictPulsing = $state(false);
  let selectedNode = $state<NodeId | null>(null);

  let rafId = 0;
  let hadLiveScan = false;
  let lastBusy = false;
  let scanStartAt = 0;

  let v4Phase: 'idle' | 'probing' | 'arriving' | 'finishing' | 'docked' = 'idle';
  let v4ArrivedAt = 0;
  let v4FromT = 0;

  let v6Phase: 'idle' | 'probing' | 'arriving' | 'finishing' | 'docked' = 'idle';
  let v6ArrivedAt = 0;
  let v6FromT = 0;

  let pendingVerdictRipple = false;
  let verdictRippleStartAt = 0;

  const DESKTOP_V6: readonly Pt[] = [
    [106, 148],
    [214, 84],
    [380, 84],
    [546, 84],
    [654, 148],
  ];
  const DESKTOP_V6_HALT: readonly Pt[] = [
    [106, 148],
    [214, 84],
    [318, 84],
  ];
  const DESKTOP_V4: readonly Pt[] = [
    [106, 148],
    [214, 212],
    [380, 212],
    [546, 212],
    [654, 148],
  ];
  const DESKTOP_V4_HALT: readonly Pt[] = [
    [106, 148],
    [214, 212],
    [318, 212],
  ];

  const MOBILE_V6: readonly Pt[] = [
    [170, 42],
    [78, 42],
    [78, 152],
    [166, 152],
    [166, 254],
  ];
  const MOBILE_V6_HALT: readonly Pt[] = [
    [170, 42],
    [78, 42],
    [78, 96],
  ];
  const MOBILE_V4: readonly Pt[] = [
    [170, 42],
    [262, 42],
    [262, 152],
    [174, 152],
    [174, 254],
  ];
  const MOBILE_V4_HALT: readonly Pt[] = [
    [170, 42],
    [262, 42],
    [262, 96],
  ];

  const NODE_POS_DESKTOP: Record<NodeId, Pt> = {
    browser: [106, 148],
    v6: [380, 84],
    v4: [380, 212],
    judge: [654, 148],
  };

  const NODE_POS_MOBILE: Record<NodeId, Pt> = {
    browser: [170, 42],
    v6: [78, 152],
    v4: [262, 152],
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

    if (v4Phase === 'probing') {
      const u = Math.min(1, (now - scanStartAt) / 850);
      v4T = easeOut(u) * 0.45;
      v4Active = true;
      keepGoing = true;
    } else if (v4Phase === 'arriving' || v4Phase === 'finishing') {
      const u = Math.min(1, (now - v4ArrivedAt) / 850);
      v4T = v4FromT + easeInOut(u) * (1 - v4FromT);
      if (u >= 1) {
        v4T = 1;
        v4Active = false;
        v4Phase = 'docked';
      } else {
        v4Active = true;
        keepGoing = true;
      }
    }

    if (v6Phase === 'probing') {
      const u = Math.min(1, (now - scanStartAt) / 900);
      v6T = easeOut(u) * 0.45;
      v6Active = true;
      keepGoing = true;
    } else if (v6Phase === 'arriving' || v6Phase === 'finishing') {
      const u = Math.min(1, (now - v6ArrivedAt) / 850);
      v6T = v6FromT + easeInOut(u) * (1 - v6FromT);
      if (u >= 1) {
        v6T = 1;
        v6Active = false;
        v6Phase = 'docked';
      } else {
        v6Active = true;
        keepGoing = true;
      }
    }

    if (pendingVerdictRipple && v4Phase === 'docked' && v6Phase === 'docked') {
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

  let activeV4 = $derived(busy && liveProgress ? liveProgress.v4 : v4);
  let activeV6 = $derived(busy && liveProgress ? liveProgress.v6 : v6);
  let v4Done = $derived(busy && liveProgress ? liveProgress.v4Done : !busy);
  let v6Done = $derived(busy && liveProgress ? liveProgress.v6Done : !busy);
  let effectiveVerdict = $derived<IPv6VerdictState>(busy ? 'unknown' : verdictState);

  let isV6Halted = $derived(!busy && !activeV6?.ip);
  let isV4Halted = $derived(!busy && !activeV4?.ip);

  $effect(() => {
    const reduced = prefersReducedMotion();
    const liveV4Ready = Boolean(liveProgress?.v4Done);
    const liveV6Ready = Boolean(liveProgress?.v6Done);

    if (busy) {
      if (!lastBusy) {
        lastBusy = true;
        hadLiveScan = true;
        scanStartAt = performance.now();
        pendingVerdictRipple = false;
        verdictPulsing = false;
        verdictPulseT = 1;

        if (reduced) {
          v4Phase = liveV4Ready ? 'docked' : 'probing';
          v4T = liveV4Ready ? 1 : 0.45;
          v4Active = false;
          v6Phase = liveV6Ready ? 'docked' : 'probing';
          v6T = liveV6Ready ? 1 : 0.45;
          v6Active = false;
          stopAnim();
          return;
        }

        v4Phase = liveV4Ready ? 'arriving' : 'probing';
        v4ArrivedAt = scanStartAt;
        v4FromT = 0;
        v4T = 0;
        v4Active = true;

        v6Phase = liveV6Ready ? 'arriving' : 'probing';
        v6ArrivedAt = scanStartAt;
        v6FromT = 0;
        v6T = 0;
        v6Active = true;
        ensureRaf();
      } else if (reduced) {
        if (liveV4Ready) {
          v4Phase = 'docked';
          v4T = 1;
        }
        if (liveV6Ready) {
          v6Phase = 'docked';
          v6T = 1;
        }
      } else {
        if (liveV4Ready && v4Phase === 'probing') {
          v4Phase = 'arriving';
          v4ArrivedAt = performance.now();
          v4FromT = v4T;
          ensureRaf();
        }
        if (liveV6Ready && v6Phase === 'probing') {
          v6Phase = 'arriving';
          v6ArrivedAt = performance.now();
          v6FromT = v6T;
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
        v4Phase = 'docked';
        v4T = 1;
        v4Active = false;
        v6Phase = 'docked';
        v6T = 1;
        v6Active = false;
        verdictPulsing = false;
        verdictPulseT = 1;
        stopAnim();
      } else {
        const now = performance.now();
        if (v4Active && !isV4Halted && v4T < 0.98) {
          v4Phase = 'finishing';
          v4ArrivedAt = now;
          v4FromT = v4T;
        } else {
          v4Phase = 'docked';
          v4T = 1;
          v4Active = false;
        }
        if (v6Active && !isV6Halted && v6T < 0.98) {
          v6Phase = 'finishing';
          v6ArrivedAt = now;
          v6FromT = v6T;
        } else {
          v6Phase = 'docked';
          v6T = 1;
          v6Active = false;
        }
        pendingVerdictRipple = true;
        ensureRaf();
      }
    } else if (!hasResult || (!pendingVerdictRipple && !verdictPulsing && v4Phase !== 'finishing' && v6Phase !== 'finishing')) {
      hadLiveScan = false;
      pendingVerdictRipple = false;
      v4Phase = 'docked';
      v4T = 1;
      v4Active = false;
      v6Phase = 'docked';
      v6T = 1;
      v6Active = false;
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
    let width = 0;
    let cutIdx = str.length;
    let overflow = false;
    for (let i = 0; i < str.length; i++) {
      const w = /[\u3000-\u9fff\uff00-\uffef]/.test(str[i]) ? 1 : 0.62;
      if (width + w > max) {
        overflow = true;
        break;
      }
      width += w;
      if (width <= max - 0.8) cutIdx = i + 1;
    }
    return overflow ? `${str.slice(0, Math.max(1, cutIdx)).trimEnd()}…` : str;
  }

  function formatDisplayIP(ip: string): string {
    return revealExit ? ip : maskIP(ip);
  }

  let isIdle = $derived(!busy && !hasResult);

  let v4Name = $derived(
    activeV4?.ip
      ? `${dnsProvider(activeV4.network) || fl.v4Idle} · ${stackRegionLabel(activeV4, locale)}`
      : isIdle
        ? fl.v4Idle
        : busy && !v4Done
          ? fl.v4Probing
          : fl.v4Disconnected,
  );
  let v4DisplayIP = $derived(
    activeV4?.ip ? formatDisplayIP(activeV4.ip) : isIdle ? m.common.idle : busy && !v4Done ? '…' : m.ipv6.page.unreachableShort,
  );

  let v6Name = $derived(
    activeV6?.ip
      ? `${dnsProvider(activeV6.network) || fl.v6Idle} · ${stackRegionLabel(activeV6, locale)}`
      : isIdle
        ? fl.v6Idle
        : busy && !v6Done
          ? fl.v6Probing
          : fl.v6Disconnected,
  );
  let v6DisplayIP = $derived(
    activeV6?.ip
      ? formatDisplayIP(activeV6.ip)
      : isIdle
        ? m.common.idle
        : busy && !v6Done
          ? '…'
          : activeV4?.ip
            ? fl.v6OnlyV4
            : m.ipv6.page.unreachableShort,
  );

  let v6Accent = $derived(
    busy
      ? v6Done
        ? activeV6?.ip
          ? 'var(--sig)'
          : 'var(--unknown)'
        : 'var(--warn)'
      : effectiveVerdict === 'leaked'
        ? 'var(--leak)'
        : isV6Halted
          ? 'var(--unknown)'
          : 'var(--sig)',
  );

  let v4Accent = $derived(
    busy
      ? v4Done
        ? activeV4?.ip
          ? 'var(--sig)'
          : 'var(--unknown)'
        : 'var(--warn)'
      : isV4Halted
        ? 'var(--unknown)'
        : 'var(--sig)',
  );

  let judgeAccent = $derived(
    busy
      ? 'var(--warn)'
      : effectiveVerdict === 'leaked'
        ? 'var(--leak)'
        : effectiveVerdict === 'clear'
          ? 'var(--sig)'
          : 'var(--unknown)',
  );

  let activeV6DesktopPts = $derived(isV6Halted ? DESKTOP_V6_HALT : DESKTOP_V6);
  let activeV6MobilePts = $derived(isV6Halted ? MOBILE_V6_HALT : MOBILE_V6);
  let activeV4DesktopPts = $derived(isV4Halted ? DESKTOP_V4_HALT : DESKTOP_V4);
  let activeV4MobilePts = $derived(isV4Halted ? MOBILE_V4_HALT : MOBILE_V4);

  let v6PosDesktop = $derived(pointAlong(activeV6DesktopPts, v6T));
  let v6Trail1Desktop = $derived(pointAlong(activeV6DesktopPts, v6T - 0.028));
  let v6Trail2Desktop = $derived(pointAlong(activeV6DesktopPts, v6T - 0.054));

  let v4PosDesktop = $derived(pointAlong(activeV4DesktopPts, v4T));
  let v4Trail1Desktop = $derived(pointAlong(activeV4DesktopPts, v4T - 0.028));
  let v4Trail2Desktop = $derived(pointAlong(activeV4DesktopPts, v4T - 0.054));

  let v6PosMobile = $derived(pointAlong(activeV6MobilePts, v6T));
  let v6Trail1Mobile = $derived(pointAlong(activeV6MobilePts, v6T - 0.028));
  let v6Trail2Mobile = $derived(pointAlong(activeV6MobilePts, v6T - 0.054));

  let v4PosMobile = $derived(pointAlong(activeV4MobilePts, v4T));
  let v4Trail1Mobile = $derived(pointAlong(activeV4MobilePts, v4T - 0.028));
  let v4Trail2Mobile = $derived(pointAlong(activeV4MobilePts, v4T - 0.054));

  let showV6Ripple = $derived(busy && Boolean(activeV6?.ip) && v6T >= 0.44);
  let showV4Ripple = $derived(busy && Boolean(activeV4?.ip) && v4T >= 0.44);
  let showJudgeRipple = $derived(
    (!busy && hasResult && !pendingVerdictRipple) ||
    (busy && liveProgress?.phase === 'comparing'),
  );

  let verdictTitleText = $derived(
    isIdle
      ? m.common.idle
      : busy
        ? liveProgress?.phase === 'comparing'
          ? fl.verdictComparing
          : fl.verdictBusy
        : m.ipv6.status[effectiveVerdict],
  );

  let statusText = $derived.by(() => {
    if (!busy) return '';
    if (!liveProgress) return fl.statusInit;
    if (liveProgress.phase === 'comparing') return fl.statusComparing;
    if (liveProgress.v4Done && !liveProgress.v6Done) return fl.statusWaitV6;
    if (!liveProgress.v4Done && liveProgress.v6Done) return fl.statusWaitV4;
    return fl.statusBoth;
  });

  let inspectorInfo = $derived.by(() => {
    if (!selectedNode) return null;
    switch (selectedNode) {
      case 'browser':
        return fl.inspectorBrowser;
      case 'v6':
        return activeV6?.ip
          ? fl.inspectorV6(v6Name, v6DisplayIP, activeV6.asn)
          : activeV4?.ip
            ? fl.inspectorV6Empty
            : fl.inspectorV6BothOffline;
      case 'v4':
        return activeV4?.ip
          ? fl.inspectorV4(v4Name, v4DisplayIP, activeV4.asn)
          : fl.inspectorV4Empty;
      case 'judge': {
        const cleanNote = verdictNote ? verdictNote.replace(/[。；.;]+$/, '') : '';
        return fl.inspectorJudge(verdictTitleText, cleanNote && !busy ? cleanNote : '');
      }
    }
  });

  let ariaSummary = $derived(
    isIdle
      ? fl.ariaIdle
      : busy && !hasResult
        ? fl.ariaBusy
        : fl.ariaResult(v4Name, v4DisplayIP, v6Name, v6DisplayIP, verdictTitleText),
  );

  let wireV6LeftLabel = $derived(fl.wireV6);
  let wireV6RightLabel = $derived(isV6Halted ? fl.wireDisconnected : locale === 'en' ? 'AAAA' : 'AAAA 出口');
  let wireV4LeftLabel = $derived(fl.wireV4);
  let wireV4RightLabel = $derived(isV4Halted ? fl.wireDisconnected : locale === 'en' ? 'A' : 'A 出口');
  let wireV6LeftW = $derived(pillWidth(wireV6LeftLabel, 44));
  let wireV6RightW = $derived(pillWidth(wireV6RightLabel, 48));
  let wireV4LeftW = $derived(pillWidth(wireV4LeftLabel, 44));
  let wireV4RightW = $derived(pillWidth(wireV4RightLabel, 48));
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

  {@const n6 = pos.v6}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.v6Aria}
    onclick={() => toggleNode('v6')}
    onkeydown={(e) => handleNodeKey(e, 'v6')}
    transform={`translate(${n6[0]}, ${n6[1]})`}
  >
    <ellipse
      cx="0"
      cy="20"
      rx="30"
      ry="11"
      fill={!busy && effectiveVerdict === 'leaked'
        ? 'rgba(248, 113, 113, 0.18)'
        : activeV6?.ip
          ? 'rgba(74, 222, 128, 0.14)'
          : 'rgba(0,0,0,0.45)'}
    />
    {#if showV6Ripple}
      <ellipse cx="0" cy="18" rx="35" ry="13" fill="none" stroke={v6Accent} stroke-width="1.6" opacity="0.72" />
    {/if}
    {#if selectedNode === 'v6'}
      <ellipse cx="0" cy="16" rx="36" ry="15" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon
      points="0,-24 22,-13 0,-2 -22,-13"
      fill={!busy && effectiveVerdict === 'leaked'
        ? `url(#${prefix}-top-leak)`
        : activeV6?.ip
          ? `url(#${prefix}-top-sig)`
          : `url(#${prefix}-top)`}
      stroke={v6Accent}
      stroke-width="0.95"
      stroke-dasharray={isV6Halted ? '3 2' : undefined}
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
    <line x1="0" y1="-13" x2="0" y2="-28" stroke={v6Accent} stroke-width="1.6" stroke-linecap="round" />
    <circle cx="0" cy="-29" r="2.8" fill={v6Accent} />
    <circle cx="-11" cy="3" r="2" fill={v6Accent} />
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
        {truncateText(v6Name, 16)}
      </text>
      <text
        x="0"
        y="51"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'leaked' ? '#f87171' : '#94a3b8'}
        font-size="10.5"
        font-family={activeV6?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}
      >
        {truncateText(v6DisplayIP, 16)}
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
        {truncateText(v6Name, 28)}
      </text>
      <text
        x="0"
        y="-37"
        text-anchor="middle"
        fill={!busy && effectiveVerdict === 'leaked' ? '#f87171' : '#94a3b8'}
        font-size="10.5"
        font-family={activeV6?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}
      >
        {activeV6?.ip ? `${truncateText(v6DisplayIP, 22)} · IPv6` : truncateText(v6DisplayIP, 26)}
      </text>
    {/if}
  </g>

  {@const n4 = pos.v4}
  <g
    class="dfs-node-hit"
    role="button"
    tabindex="0"
    aria-label={fl.v4Aria}
    onclick={() => toggleNode('v4')}
    onkeydown={(e) => handleNodeKey(e, 'v4')}
    transform={`translate(${n4[0]}, ${n4[1]})`}
  >
    <ellipse cx="0" cy="20" rx="28" ry="10" fill={activeV4?.ip ? 'rgba(74, 222, 128, 0.14)' : 'rgba(0,0,0,0.48)'} />
    {#if showV4Ripple}
      <ellipse cx="0" cy="18" rx="34" ry="13" fill="none" stroke="var(--sig)" stroke-width="1.5" opacity="0.65" />
    {/if}
    {#if selectedNode === 'v4'}
      <ellipse cx="0" cy="16" rx="35" ry="14" fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon
      points="0,-28 21,-18 0,-8 -21,-18"
      fill={activeV4?.ip ? `url(#${prefix}-top-sig)` : `url(#${prefix}-top)`}
      stroke={v4Accent}
      stroke-width="0.9"
      stroke-dasharray={isV4Halted ? '3 2' : undefined}
      stroke-opacity={activeV4?.ip ? 0.78 : 0.45}
    />
    <polygon points="-21,-18 0,-8 0,18 -21,8" fill={`url(#${prefix}-left)`} stroke="#334155" stroke-width="0.85" />
    <polygon points="0,-8 21,-18 21,8 0,18" fill={`url(#${prefix}-right)`} stroke="#1e293b" stroke-width="0.85" />
    <line x1="-16" y1="-10" x2="-4" y2="-4" stroke={v4Accent} stroke-width="1.8" stroke-linecap="round" opacity={activeV4?.ip ? 0.92 : 0.4} />
    <line x1="-16" y1="-2" x2="-4" y2="4" stroke="#38bdf8" stroke-width="1.4" stroke-linecap="round" opacity="0.55" />
    <line x1="-16" y1="6" x2="-4" y2="12" stroke="#64748b" stroke-width="1.4" stroke-linecap="round" opacity="0.55" />
    <circle cx="0" cy="0" r="30" fill="transparent" />
    {#if compact}
      <text x="0" y="36" text-anchor="middle" fill="#f8fafc" font-size="11.5" font-weight="600">{truncateText(v4Name, 16)}</text>
      <text x="0" y="51" text-anchor="middle" fill="#94a3b8" font-size="10.5" font-family={activeV4?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}>{truncateText(v4DisplayIP, 16)}</text>
    {:else}
      <text x="0" y="38" text-anchor="middle" fill="#f8fafc" font-size="12" font-weight="600">{truncateText(v4Name, 26)}</text>
      <text x="0" y="53" text-anchor="middle" fill="#94a3b8" font-size="10.5" font-family={activeV4?.ip ? "'JetBrains Mono', ui-monospace, monospace" : undefined}>{activeV4?.ip ? `${v4DisplayIP} · IPv4` : v4DisplayIP}</text>
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
      {@const pulseRx = verdictPulsing ? (compact ? 22 + 12 * verdictPulseT : 26 + 15 * verdictPulseT) : compact ? 34 : 38}
      {@const pulseRy = verdictPulsing ? (compact ? 8 + 4.5 * verdictPulseT : 10 + 6.5 * verdictPulseT) : compact ? 12 : 16}
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
      <ellipse cx="0" cy="14" rx={compact ? 24 : 28} ry={compact ? 9 : 11.5} fill="none" stroke={judgeAccent} stroke-width="1.2" opacity="0.85" />
    {/if}
    {#if selectedNode === 'judge'}
      <ellipse cx="0" cy="14" rx={compact ? 36 : 40} ry={compact ? 13 : 17} fill="none" stroke="var(--radar-400)" stroke-width="1.4" stroke-dasharray="4 3" />
    {/if}
    <polygon points="0,-10 24,2 0,14 -24,2" fill={`url(#${prefix}-top)`} stroke={judgeAccent} stroke-width="0.9" stroke-opacity="0.7" />
    <polygon points="-24,2 0,14 0,20 -24,8" fill={`url(#${prefix}-left)`} stroke="#334155" stroke-width="0.8" />
    <polygon points="0,14 24,2 24,8 0,20" fill={`url(#${prefix}-right)`} stroke="#1e293b" stroke-width="0.8" />
    <circle cx="0" cy="-8" r="11" fill="#040d1d" stroke={judgeAccent} stroke-width="1.4" />
    <circle cx="0" cy="-8" r="4.5" fill={judgeAccent} opacity="0.95" />
    <path d="M 0 -22 V -16 M 0 0 V 6 M -14 -8 H -8 M 8 -8 H 14" stroke={judgeAccent} stroke-width="1.5" stroke-linecap="round" />
    <circle cx="0" cy="0" r="32" fill="transparent" />
    <text x="0" y={compact ? 40 : 44} text-anchor="middle" fill="#f8fafc" font-size={compact ? 11.5 : 12} font-weight="600" paint-order="stroke" stroke="#030917" stroke-width="4" stroke-linejoin="round">{fl.judgeNode}</text>
    <text x="0" y={compact ? 54 : 59} text-anchor="middle" fill={judgeAccent} font-size="10.5" font-weight="600" paint-order="stroke" stroke="#030917" stroke-width="3.5" stroke-linejoin="round">
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
        style:background={v6Accent}
        style:--_soft="transparent"
      ></span>
      <span>{statusText}</span>
    </div>
  {/if}

  <div class="dfs-canvas-wrap">
    <svg class="dfs-svg-desktop" viewBox="0 0 760 288">
      {@render svgDefs('v6d')}
      <rect width="760" height="288" fill="url(#v6d-iso)" />

      <path
        d={polyToD(DESKTOP_V6)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isV6Halted ? '5 4' : undefined}
      />
      <path
        d={polyToD(DESKTOP_V4)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isV4Halted ? '5 4' : undefined}
      />

      <path
        d={polyToD(activeV6DesktopPts)}
        fill="none"
        stroke={v6Accent}
        stroke-width={busy ? 2.5 : 2.3}
        stroke-opacity={busy && !v6Done ? 0.55 : 0.82}
        stroke-dasharray={isV6Halted ? '5 4' : undefined}
      />
      <path
        d={polyToD(activeV4DesktopPts)}
        fill="none"
        stroke={v4Accent}
        stroke-width={activeV4?.ip ? 2.4 : 1.8}
        stroke-opacity={activeV4?.ip ? 0.82 : 0.45}
        stroke-dasharray={isV4Halted ? '5 4' : undefined}
      />

      <g transform="translate(252, 106)" opacity="0.88">
        <rect x={-wireV6LeftW / 2} y="-9.5" width={wireV6LeftW} height="19" rx="9.5" fill="#050d1d" stroke={v6Accent} stroke-opacity="0.4" />
        <text x="0" y="3.2" text-anchor="middle" fill="#cbd5e1" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">{wireV6LeftLabel}</text>
      </g>
      <g transform="translate(508, 106)" opacity="0.88">
        <rect x={-wireV6RightW / 2} y="-9.5" width={wireV6RightW} height="19" rx="9.5" fill="#050d1d" stroke="rgba(148,163,184,0.28)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#cbd5e1" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">
          {wireV6RightLabel}
        </text>
      </g>
      <g transform="translate(252, 190)" opacity="0.88">
        <rect x={-wireV4LeftW / 2} y="-9.5" width={wireV4LeftW} height="19" rx="9.5" fill="#050d1d" stroke="rgba(74,222,128,0.38)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#86efac" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">{wireV4LeftLabel}</text>
      </g>
      <g transform="translate(508, 190)" opacity="0.88">
        <rect x={-wireV4RightW / 2} y="-9.5" width={wireV4RightW} height="19" rx="9.5" fill="#050d1d" stroke="rgba(74,222,128,0.28)" />
        <text x="0" y="3.2" text-anchor="middle" fill="#86efac" font-size="9.5" font-family="'JetBrains Mono', ui-monospace, monospace">
          {wireV4RightLabel}
        </text>
      </g>

      {#if isV6Halted}
        <text x="324" y="88" text-anchor="middle" fill="var(--unknown)" font-size="14" font-weight="700">×</text>
      {/if}
      {#if isV4Halted}
        <text x="324" y="216" text-anchor="middle" fill="var(--unknown)" font-size="14" font-weight="700">×</text>
      {/if}

      {@render isoNodes('v6d', NODE_POS_DESKTOP, false)}

      {#if v4Active && v4T > 0.05}
        {#if v4T > 0.09 && v4T < 0.98}
          <circle cx={v4Trail2Desktop.x} cy={v4Trail2Desktop.y} r="2.2" fill={v4Accent} opacity="0.24" />
          <circle cx={v4Trail1Desktop.x} cy={v4Trail1Desktop.y} r="3.2" fill={v4Accent} opacity="0.46" />
        {/if}
        <g transform={`translate(${v4PosDesktop.x}, ${v4PosDesktop.y})`} filter="url(#v6d-glow)">
          <rect x="-24" y="-11" width="48" height="22" rx="11" fill={v4Accent} opacity="0.22" />
          <rect x="-22" y="-9" width="44" height="18" rx="9" fill="#071122" stroke={v4Accent} stroke-width="1.45" />
          <text x="0" y="3.3" text-anchor="middle" fill="#f8fafc" font-size="9.5" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">IPv4</text>
        </g>
      {/if}

      {#if v6Active && v6T > 0.05}
        {#if v6T > 0.09 && v6T < 0.98}
          <circle cx={v6Trail2Desktop.x} cy={v6Trail2Desktop.y} r="2.2" fill={v6Accent} opacity="0.24" />
          <circle cx={v6Trail1Desktop.x} cy={v6Trail1Desktop.y} r="3.2" fill={v6Accent} opacity="0.46" />
        {/if}
        <g transform={`translate(${v6PosDesktop.x}, ${v6PosDesktop.y})`} filter="url(#v6d-glow)">
          <rect x="-24" y="-11" width="48" height="22" rx="11" fill={v6Accent} opacity="0.22" />
          <rect x="-22" y="-9" width="44" height="18" rx="9" fill="#071122" stroke={v6Accent} stroke-width="1.45" />
          <text x="0" y="3.3" text-anchor="middle" fill="#f8fafc" font-size="9.5" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">IPv6</text>
        </g>
      {/if}
    </svg>

    <svg class="dfs-svg-mobile" viewBox="0 0 340 314">
      {@render svgDefs('v6m')}
      <rect width="340" height="314" fill="url(#v6m-iso)" />

      <path
        d={polyToD(MOBILE_V6)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isV6Halted ? '5 4' : undefined}
      />
      <path
        d={polyToD(MOBILE_V4)}
        fill="none"
        stroke="rgba(148,163,184,0.16)"
        stroke-width="2"
        stroke-dasharray={isV4Halted ? '5 4' : undefined}
      />

      <path
        d={polyToD(activeV6MobilePts)}
        fill="none"
        stroke={v6Accent}
        stroke-width={busy ? 2.5 : 2.3}
        stroke-opacity={busy && !v6Done ? 0.55 : 0.82}
        stroke-dasharray={isV6Halted ? '5 4' : undefined}
      />
      <path
        d={polyToD(activeV4MobilePts)}
        fill="none"
        stroke={v4Accent}
        stroke-width={activeV4?.ip ? 2.4 : 1.8}
        stroke-opacity={activeV4?.ip ? 0.82 : 0.45}
        stroke-dasharray={isV4Halted ? '5 4' : undefined}
      />

      <g transform="translate(114, 102)" opacity="0.88">
        <rect x="-21" y="-8.5" width="42" height="17" rx="8.5" fill="#050d1d" stroke={v6Accent} stroke-opacity="0.4" />
        <text x="0" y="3" text-anchor="middle" fill="#cbd5e1" font-size="9" font-family="'JetBrains Mono', ui-monospace, monospace">IPv6</text>
      </g>
      <g transform="translate(226, 102)" opacity="0.88">
        <rect x="-21" y="-8.5" width="42" height="17" rx="8.5" fill="#050d1d" stroke="rgba(74,222,128,0.38)" />
        <text x="0" y="3" text-anchor="middle" fill="#86efac" font-size="9" font-family="'JetBrains Mono', ui-monospace, monospace">IPv4</text>
      </g>

      {#if isV6Halted}
        <text x="78" y="106" text-anchor="middle" fill="var(--unknown)" font-size="13" font-weight="700">×</text>
      {/if}
      {#if isV4Halted}
        <text x="262" y="106" text-anchor="middle" fill="var(--unknown)" font-size="13" font-weight="700">×</text>
      {/if}

      {@render isoNodes('v6m', NODE_POS_MOBILE, true)}

      {#if v4Active && v4T > 0.05}
        {#if v4T > 0.09 && v4T < 0.98}
          <circle cx={v4Trail2Mobile.x} cy={v4Trail2Mobile.y} r="2" fill={v4Accent} opacity="0.24" />
          <circle cx={v4Trail1Mobile.x} cy={v4Trail1Mobile.y} r="3" fill={v4Accent} opacity="0.46" />
        {/if}
        <g transform={`translate(${v4PosMobile.x}, ${v4PosMobile.y})`} filter="url(#v6m-glow)">
          <rect x="-22" y="-10" width="44" height="20" rx="10" fill={v4Accent} opacity="0.24" />
          <rect x="-20" y="-8" width="40" height="16" rx="8" fill="#071122" stroke={v4Accent} stroke-width="1.4" />
          <text x="0" y="3" text-anchor="middle" fill="#f8fafc" font-size="8.8" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">IPv4</text>
        </g>
      {/if}

      {#if v6Active && v6T > 0.05}
        {#if v6T > 0.09 && v6T < 0.98}
          <circle cx={v6Trail2Mobile.x} cy={v6Trail2Mobile.y} r="2" fill={v6Accent} opacity="0.24" />
          <circle cx={v6Trail1Mobile.x} cy={v6Trail1Mobile.y} r="3" fill={v6Accent} opacity="0.46" />
        {/if}
        <g transform={`translate(${v6PosMobile.x}, ${v6PosMobile.y})`} filter="url(#v6m-glow)">
          <rect x="-22" y="-10" width="44" height="20" rx="10" fill={v6Accent} opacity="0.24" />
          <rect x="-20" y="-8" width="40" height="16" rx="8" fill="#071122" stroke={v6Accent} stroke-width="1.4" />
          <text x="0" y="3" text-anchor="middle" fill="#f8fafc" font-size="8.8" font-weight="700" font-family="'JetBrains Mono', ui-monospace, monospace">IPv6</text>
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
