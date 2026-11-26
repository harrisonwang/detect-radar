<script lang="ts">
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import { getMessages, type Locale } from '../../i18n';
  import type { ReportGroup, SignalState } from '../../lib/report';
  import { stateVar } from '../../lib/report';

  interface Props {
    locale?: Locale;
    groups: ReportGroup[];
    verdictState: SignalState;
    riskLabel: string;
    isScanning?: boolean;
    pendingKeys?: string[];
    settledKeys?: string[];
    activeKey?: string | null;
    scanStageLabel?: string;
  }
  let {
    locale = 'zh-CN',
    groups,
    verdictState,
    riskLabel,
    isScanning = false,
    pendingKeys = [],
    settledKeys = [],
    activeKey = null,
    scanStageLabel = '',
  }: Props = $props();

  let m = $derived(getMessages(locale));
  const isPending = (key: string) => pendingKeys.includes(key);
  const isSettled = (key: string) => settledKeys.includes(key);

  const C = 180;
  const CY = 124;
  const R = 78;
  const rings = [0.33, 0.66, 1];

  const angleFor = (i: number) => (-90 + i * 90) * (Math.PI / 180);
  const pt = (i: number, r: number) => ({
    x: C + r * Math.cos(angleFor(i)),
    y: CY + r * Math.sin(angleFor(i)),
  });

  let axisEnds = $derived(groups.map((_, i) => pt(i, R)));
  let targetScores = $derived(
    groups.map((g) => (isSettled(g.key) && !isPending(g.key) ? Math.max(5, g.score) : 5)),
  );
  const twScores = Tween.of(() => targetScores, { duration: 600, easing: cubicOut });
  let vertices = $derived(twScores.current.map((s, i) => pt(i, (s / 100) * R)));
  let polygon = $derived(vertices.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
  let labelOffset = $derived(locale === 'en' ? 16 : 22);
  let labelPos = $derived(groups.map((_, i) => pt(i, R + labelOffset)));
  const labelAnchor = (i: number) => (i === 1 ? 'start' : i === 3 ? 'end' : 'middle');
</script>

<div class="glass-card rounded-xl p-5 h-full flex flex-col items-center justify-center">
  <div class="relative w-full max-w-80">
    <svg viewBox="0 0 360 256" class="w-full h-auto overflow-visible" role="img" aria-label={m.home.radarAria}>
      {#each rings as r}
        <circle cx={C} cy={CY} r={R * r} fill="none" stroke="var(--line)" stroke-width="1" />
      {/each}
      {#each axisEnds as e, i}
        <line x1={C} y1={CY} x2={e.x} y2={e.y} stroke={activeKey === groups[i]?.key ? 'var(--radar-400)' : 'var(--line)'} stroke-width={activeKey === groups[i]?.key ? '2' : '1'} opacity={activeKey === groups[i]?.key ? '0.9' : '1'} />
      {/each}

      {#if settledKeys.length > 0 || !isScanning}
        <polygon
          points={polygon}
          fill={stateVar(verdictState)} fill-opacity="0.16"
          stroke={stateVar(verdictState)} stroke-width="1.5" stroke-linejoin="round" class="radar-polygon"
        />
        {#each vertices as v, i}
          {#if isSettled(groups[i].key) && !isPending(groups[i].key)}
            <circle cx={v.x} cy={v.y} r="3.5" fill={stateVar(groups[i].state)} stroke="var(--space-950)" stroke-width="1.5" class="radar-node" />
          {/if}
        {/each}
      {/if}

      {#each groups as g, i}
        <text
          x={labelPos[i].x}
          y={i === 0 ? labelPos[i].y - 10 : i === 2 ? labelPos[i].y + 2 : labelPos[i].y - 4}
          text-anchor={labelAnchor(i)}
          dominant-baseline="middle"
          fill={activeKey === g.key ? 'var(--radar-400)' : 'var(--slate-400)'}
          font-size={locale === 'en' ? (activeKey === g.key ? '11.5' : '11') : (activeKey === g.key ? '13' : '12')}
          class="font-mono"
        >{m.home.radarShort[g.key] ?? g.label}</text>
        <text
          x={labelPos[i].x}
          y={i === 0 ? labelPos[i].y + 4 : i === 2 ? labelPos[i].y + 16 : labelPos[i].y + 11}
          text-anchor={labelAnchor(i)}
          dominant-baseline="middle"
          fill={activeKey === g.key ? 'var(--radar-400)' : stateVar(isPending(g.key) || !isSettled(g.key) ? 'unknown' : g.state)}
          font-size="11"
          class="tnum"
        >{isPending(g.key) || !isSettled(g.key) ? '··' : g.score}</text>
      {/each}
    </svg>

    {#if isScanning}
      <div class="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div class="w-40 h-40 radar-scan animate-radar-spin rounded-full opacity-50"></div>
      </div>
    {/if}
  </div>

  <div class="mt-4 flex items-center gap-2">
    <span class="text-xs text-(--slate-500)">{m.home.overallRisk}</span>
    <span class="text-sm font-semibold" style="color:{stateVar(verdictState)}">{isScanning ? m.common.scanning : riskLabel}</span>
  </div>
  {#if scanStageLabel && (isScanning || activeKey === 'leak')}
    <div class="mt-2 text-xs text-(--slate-400) font-mono animate-pulse text-center">{scanStageLabel}</div>
  {/if}
</div>
