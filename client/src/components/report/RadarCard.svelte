<script lang="ts">
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import type { ReportGroup, SignalState } from '../../lib/report';
  import { stateVar } from '../../lib/report';

  interface Props {
    groups: ReportGroup[];
    verdictState: SignalState;
    riskLabel: string;
    isScanning?: boolean;
    /** 结果尚未返回的组（如慢路径的 DNS 泄露）：该轴显示待定，顶点收在中心 */
    pendingKeys?: string[];
  }
  let { groups, verdictState, riskLabel, isScanning = false, pendingKeys = [] }: Props = $props();

  const isPending = (key: string) => pendingKeys.includes(key);

  const C = 150;
  const CY = 120;
  const R = 82;
  const rings = [0.33, 0.66, 1];
  // 雷达轴用短名（明细卡才用全名）
  const SHORT: Record<string, string> = {
    identity: '网络身份', consistency: '环境一致性', leak: 'DNS/IP 泄露', environment: '环境指纹',
  };

  const angleFor = (i: number) => (-90 + i * 90) * (Math.PI / 180);
  const pt = (i: number, r: number) => ({
    x: C + r * Math.cos(angleFor(i)),
    y: CY + r * Math.sin(angleFor(i)),
  });

  let axisEnds = $derived(groups.map((_, i) => pt(i, R)));
  // 分数经 Tween 过渡再算顶点：扫描中/待定轴收在中心，结果落地后平滑展开
  let targetScores = $derived(
    groups.map((g) => (isScanning || isPending(g.key) ? 5 : Math.max(5, g.score))),
  );
  const twScores = Tween.of(() => targetScores, { duration: 600, easing: cubicOut });
  let vertices = $derived(twScores.current.map((s, i) => pt(i, (s / 100) * R)));
  let polygon = $derived(vertices.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '));
  let labelPos = $derived(groups.map((_, i) => pt(i, R + 26)));
  const labelAnchor = (i: number) => (i === 1 ? 'start' : i === 3 ? 'end' : 'middle');
</script>

<div class="glass-card rounded-xl p-5 h-full flex flex-col items-center justify-center reveal">
  <div class="relative w-full max-w-75">
    <svg viewBox="0 0 300 260" class="w-full h-auto overflow-visible" role="img" aria-label="检测雷达">
      {#each rings as r}
        <circle cx={C} cy={CY} r={R * r} fill="none" stroke="var(--line)" stroke-width="1" />
      {/each}
      {#each axisEnds as e}
        <line x1={C} y1={CY} x2={e.x} y2={e.y} stroke="var(--line)" stroke-width="1" />
      {/each}

      {#if !isScanning}
        <polygon
          points={polygon}
          fill={stateVar(verdictState)} fill-opacity="0.16"
          stroke={stateVar(verdictState)} stroke-width="1.5" stroke-linejoin="round"
        />
        {#each vertices as v, i}
          <circle cx={v.x} cy={v.y} r="3.5" fill={stateVar(isPending(groups[i].key) ? 'unknown' : groups[i].state)} stroke="var(--space-950)" stroke-width="1.5" />
        {/each}
      {/if}

      {#each groups as g, i}
        <text x={labelPos[i].x} y={labelPos[i].y} text-anchor={labelAnchor(i)} dominant-baseline="middle" fill="var(--slate-400)" font-size="12" class="font-mono">{SHORT[g.key] ?? g.label}</text>
        <text x={labelPos[i].x} y={labelPos[i].y + 14} text-anchor={labelAnchor(i)} dominant-baseline="middle" fill={stateVar(isPending(g.key) ? 'unknown' : g.state)} font-size="11" class="tnum">{isScanning || isPending(g.key) ? '··' : g.score}</text>
      {/each}
    </svg>

    {#if isScanning}
      <div class="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div class="w-44 h-44 radar-scan animate-radar-spin rounded-full opacity-50"></div>
      </div>
    {/if}
  </div>

  <div class="mt-4 flex items-center gap-2">
    <span class="text-xs text-(--slate-500)">综合风险</span>
    <span class="text-sm font-semibold" style="color:{stateVar(verdictState)}">{isScanning ? '检测中' : riskLabel}</span>
  </div>
</div>
