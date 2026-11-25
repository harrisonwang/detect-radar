<script lang="ts">
  import type { ReadoutRow, SignalState } from '../../lib/report';
  import { stateVar, stateSoft } from '../../lib/report';
  import { specialistToolFromHref, trackEvent } from '../../lib/analytics';

  interface Props {
    title: string;
    icon: string;
    rows: ReadoutRow[];
    state: SignalState;
    note?: string;
    isScanning?: boolean;
    settled?: boolean;
    delay?: number;
    active?: boolean;
  }
  let { title, icon, rows, state, note = '', isScanning = false, settled = false, delay = 0, active = false }: Props = $props();
</script>

<div class="glass-card rounded-xl p-5 h-full reveal" class:scan-focus={active} class:scan-settle={settled} style="animation-delay:{delay}ms">
  <header class="flex items-center justify-between mb-3.5">
    <div class="flex items-center gap-2.5">
      <i class="fa-solid {icon} text-(--slate-400) text-sm"></i>
      <p class="text-sm font-semibold text-white tracking-wide">{title}</p>
    </div>
    <span class="dot" style="background:{stateVar(state)}; --_soft:{stateSoft(state)}"></span>
  </header>

  {#if isScanning && !settled}
    <div class="space-y-3.5">
      {#each Array(3) as _}
        <div class="flex items-center justify-between">
          <div class="h-3.5 w-16 bg-slate-800 rounded animate-pulse"></div>
          <div class="h-3.5 w-20 bg-slate-800/60 rounded animate-pulse"></div>
        </div>
      {/each}
    </div>
  {:else if rows.length === 0}
    <p class="text-sm text-(--slate-500)">待检测</p>
  {:else}
    <div class="space-y-3">
      {#each rows as r}
        <div class="flex items-center gap-2.5">
          <span class="dot" style="background:{stateVar(r.state)}; --_soft:{stateSoft(r.state)}"></span>
          <span class="text-sm text-(--slate-400) shrink-0">{r.label}</span>
          <div class="text-right flex-1 min-w-0">
            {#if r.compare}
              <span class="text-sm text-(--slate-200)">
                <span class="text-[10px] text-(--slate-500) font-mono">浏览器</span>
                {r.compare.browser}
                <span class="text-(--slate-600) mx-0.5">↔</span>
                <span class="text-[10px] text-(--slate-500) font-mono">IP</span>
                {r.compare.ip}
              </span>
            {:else if r.href}
              <a
                href={r.href}
                aria-label={`${r.label} ${r.value}`}
                onclick={() => {
                  const to = specialistToolFromHref(r.href);
                  if (to) trackEvent('tool-open', { from: 'home', to, source: 'readout' });
                }}
                class="text-sm text-(--slate-200) underline decoration-slate-600 underline-offset-4 hover:text-(--radar-400)"
              >{r.value}</a>
            {:else}
              <span class="text-sm text-(--slate-200) tnum">{r.value}</span>
            {/if}

          </div>
        </div>
      {/each}
    </div>

  {/if}
</div>
