<script lang="ts">
  import type { ReportModel, Severity } from '../../lib/report';
  import { stateVar, stateSoft } from '../../lib/report';

  interface Props {
    report: ReportModel | null;
    isScanning?: boolean;
  }
  let { report, isScanning = false }: Props = $props();

  const SEV: Record<Severity, { color: string; soft: string; label: string; icon: string }> = {
    leak: { color: 'var(--leak)', soft: 'var(--leak-soft)', label: '暴露', icon: 'fa-triangle-exclamation' },
    contradiction: { color: 'var(--warn)', soft: 'var(--warn-soft)', label: '冲突', icon: 'fa-not-equal' },
    caution: { color: 'var(--warn)', soft: 'var(--warn-soft)', label: '留意', icon: 'fa-circle-exclamation' },
    info: { color: 'var(--unknown)', soft: 'var(--unknown-soft)', label: '提示', icon: 'fa-circle-info' },
  };
</script>

<div class="glass-card rounded-xl p-6 h-full flex flex-col reveal">
  {#if isScanning}
    <div class="space-y-3">
      <div class="h-6 w-2/5 bg-slate-800 rounded animate-pulse"></div>
      <div class="h-4 w-4/5 bg-slate-800/70 rounded animate-pulse"></div>
      <div class="h-4 w-3/5 bg-slate-800/50 rounded animate-pulse mt-6"></div>
    </div>
  {:else if report}
    <!-- 判定 -->
    <div class="flex items-start gap-3">
      <span class="dot mt-2" style="background:{stateVar(report.verdict.state)}; --_soft:{stateSoft(report.verdict.state)}"></span>
      <div>
        <h2 class="text-xl md:text-2xl font-bold text-white leading-snug">{report.verdict.headline}</h2>
        <p class="text-sm text-(--slate-400) mt-1.5 leading-relaxed max-w-xl">{report.verdict.sub}</p>
      </div>
    </div>

    <!-- 暴露项 -->
    <div class="mt-5 pt-4 border-t border-slate-800 flex-1">
      {#if report.findings.length === 0}
        <div class="flex items-center gap-2 text-sm text-(--slate-300)">
          <i class="fa-solid fa-circle-check" style="color:var(--sig)"></i> 未见明显异常
        </div>
      {:else}
        <div class="flex items-center justify-between mb-3">
          <span class="text-xs font-mono text-(--slate-500) tracking-wider">暴露项</span>
          <span class="text-xs font-mono text-(--slate-600)">{report.findings.length}</span>
        </div>
        <ul class="space-y-2.5">
          {#each report.findings as f}
            {@const m = SEV[f.severity]}
            <li class="flex items-start gap-2.5">
              <span class="text-[10px] font-mono px-1.5 py-1 rounded shrink-0 flex items-center gap-1" style="color:{m.color}; background:{m.soft}">
                <i class="fa-solid {m.icon}"></i>{m.label}
              </span>
              <div class="min-w-0 flex-1">
                <span class="text-sm text-(--slate-200) leading-snug">{f.title}</span>
                <!-- 窄屏放开换行：单行省略号会把暴露项的证据（被比对的两个值）截掉一半 -->
                <span class="text-xs text-(--slate-500) block sm:truncate mt-0.5">{f.fact}</span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
