<script lang="ts">
  import type { IdentityReadout, RTTReadout, SignalState } from '../../lib/report';
  import { stateVar, stateSoft } from '../../lib/report';
  import ExitIPAddress from './ExitIPAddress.svelte';

  interface Props {
    identity: IdentityReadout;
    rtt?: RTTReadout;
    isScanning?: boolean;
    idle?: boolean;
    settled?: boolean;
    revealed?: boolean;
    active?: boolean;
    onToggleReveal?: () => void;
    onCopyFeedback?: (message: string) => void;
  }
  let { identity, rtt, isScanning = false, idle = false, settled = false, revealed = false, active = false, onToggleReveal, onCopyFeedback }: Props = $props();

  const ms = (x: number) => x.toFixed(1);

  function flag(code: string): string {
    if (!code || code.length !== 2) return '';
    const cc = code.toUpperCase();
    if (!/^[A-Z]{2}$/.test(cc)) return '';
    return String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
  }

  function typeIcon(label: string): string {
    if (label.includes('机房')) return 'fa-server';
    if (label.includes('住宅')) return 'fa-house';
    if (label.includes('移动')) return 'fa-mobile-screen';
    return 'fa-circle-question';
  }

  let fraudWord = $derived(
    identity.fraudScore < 25 ? '很低' : identity.fraudScore < 50 ? '较低' : identity.fraudScore < 75 ? '偏高' : '高危',
  );
  let fraudColor = $derived(identity.fraudScore >= 50 ? 'var(--leak)' : identity.fraudScore >= 25 ? 'var(--warn)' : 'var(--sig)');

  let humanPct = $derived(
    typeof identity.humanRatio === 'number' && Number.isFinite(identity.humanRatio)
      ? Math.round(Math.max(0, Math.min(100, identity.humanRatio)) * 10) / 10
      : null,
  );
  let botPct = $derived(humanPct != null ? Math.round((100 - humanPct) * 10) / 10 : null);
  let humanWord = $derived(
    humanPct == null ? '' : humanPct >= 70 ? '较高' : humanPct >= 40 ? '中等' : '偏低',
  );
  let humanColor = $derived(
    humanPct == null ? 'var(--unknown)' : humanPct >= 70 ? 'var(--sig)' : humanPct >= 40 ? 'var(--warn)' : 'var(--leak)',
  );
  let asnDisplay = $derived.by(() => {
    const raw = (identity.asn ?? '').trim();
    if (!raw || raw === '—') return '—';
    if (/^as\d+$/i.test(raw)) return raw.toUpperCase();
    if (/^\d+$/.test(raw)) return `AS${raw}`;
    return raw;
  });
  let usageBadgeState = $derived<SignalState>(
    identity.state === 'coherent' && identity.usageLabel.includes('机房') ? 'unknown' : identity.state,
  );
  const pct = (x: number) => (Number.isInteger(x) ? String(x) : x.toFixed(1));
</script>

<div class="glass-card rounded-xl p-6" class:scan-focus={active}>
  <header class="flex items-center justify-between mb-5">
    <div class="flex items-center gap-2.5">
      <i class="fa-solid fa-id-card text-(--radar-400)"></i>
      <p class="text-sm font-semibold text-white tracking-wide">网络身份</p>
    </div>
    <span class="dot" style="background:{stateVar(identity.state)}; --_soft:{stateSoft(identity.state)}"></span>
  </header>

  {#if isScanning && !settled}
    <div class="flex flex-col md:flex-row gap-6">
      <div class="md:w-[30%] space-y-3">
        <div class="h-8 w-44 bg-slate-800 rounded animate-pulse"></div>
        <div class="h-6 w-24 bg-slate-800/60 rounded animate-pulse"></div>
      </div>
      <div class="flex-1 grid grid-cols-2 lg:grid-cols-3 gap-4">
        {#each Array(6) as _}<div class="h-10 bg-slate-800/50 rounded animate-pulse"></div>{/each}
      </div>
    </div>
  {:else if idle}
    <p class="text-sm text-(--slate-500)">待检测</p>
  {:else}
    <div class:scan-settle={settled}>
    <div class="flex flex-col md:flex-row gap-6">
      <div class="md:w-[30%] md:border-r border-slate-800 md:pr-6 flex flex-col justify-center min-w-0">
        <div class="text-xs text-(--slate-500) mb-1.5">出口 IP</div>
        <ExitIPAddress ip={identity.ip} {revealed} {onToggleReveal} {onCopyFeedback} />
        <div class="mt-3.5">
          <span
            class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-md"
            style="color:{stateVar(usageBadgeState)}; background:{stateSoft(usageBadgeState)}"
          >
            <i class="fa-solid {typeIcon(identity.usageLabel)}"></i>{identity.usageLabel}
          </span>
        </div>
      </div>

      <div class="flex-1 grid grid-cols-2 lg:grid-cols-3 gap-x-6 gap-y-5 min-w-0">
        <div class="min-w-0">
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-location-dot text-[10px]"></i> 归属地
          </div>
          <div class="text-sm text-(--slate-200) truncate">
            {#if flag(identity.countryCode)}<span class="mr-1 emoji">{flag(identity.countryCode)}</span>{/if}{identity.geo}
          </div>
        </div>

        <div class="min-w-0">
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-building text-[10px]"></i> 运营商
          </div>
          <div class="text-sm text-(--slate-200) truncate" title={identity.org}>{identity.org}</div>
        </div>

        <div class="min-w-0">
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-network-wired text-[10px]"></i> ASN
          </div>
          <div class="text-sm font-mono text-(--slate-200) truncate" title={asnDisplay}>{asnDisplay}</div>
        </div>

        <div class="min-w-0">
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-shield-halved text-[10px]"></i> 欺诈评分
          </div>
          <div class="text-sm whitespace-nowrap">
            <span class="tnum text-(--slate-200)">{identity.fraudScore}</span>
            <span class="text-(--slate-600)"> / 100</span>
            <span class="ml-1" style="color:{fraudColor}">({fraudWord})</span>
          </div>
        </div>

        <div class="min-w-0">
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-list-check text-[10px]"></i> 黑名单
          </div>
          <div class="text-sm flex items-center gap-1.5 whitespace-nowrap">
            <span class="dot" style="background:{identity.blacklist.length ? 'var(--leak)' : 'var(--sig)'}; --_soft:transparent"></span>
            <span style="color:{identity.blacklist.length ? 'var(--leak)' : 'var(--sig)'}">{identity.blacklist.length ? `命中 ${identity.blacklist.length}` : '未命中'}</span>
          </div>
        </div>

        <div
          class="min-w-0"
          title={humanPct != null && botPct != null
            ? `人类 ${pct(humanPct)}% / 机器人 ${pct(botPct)}% · Cloudflare Radar 网段统计`
            : 'Cloudflare Radar 暂无该网段数据'}
        >
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-users text-[10px]"></i> 人类流量
            <span class="text-[10px] text-(--slate-600)">近30天</span>
          </div>
          {#if humanPct != null}
            <div class="text-sm flex items-center gap-1.5 whitespace-nowrap">
              <span class="dot" style="background:{humanColor}; --_soft:transparent"></span>
              <span class="tnum text-(--slate-200)">{pct(humanPct)}%</span>
              <span style="color:{humanColor}">({humanWord})</span>
            </div>
          {:else}
            <div class="text-sm text-(--slate-500)">暂无数据</div>
          {/if}
        </div>
      </div>
    </div>

    {#if identity.ceilingNote}
      <p class="mt-5 text-xs text-(--slate-500) leading-relaxed border-l-2 pl-3" style="border-color:var(--unknown)">{identity.ceilingNote}</p>
    {/if}

    {#if identity.flags.length || identity.exposure.length}
      <div class="mt-3 flex flex-wrap gap-1.5">
        {#each identity.flags as f}
          <span class="text-[10px] font-mono px-1.5 py-0.5 rounded" style="color:var(--leak); background:var(--leak-soft)">{f}</span>
        {/each}
        {#each identity.exposure as e}
          <span class="text-[10px] font-mono px-1.5 py-0.5 rounded" style="color:var(--warn); background:var(--warn-soft)">{e}</span>
        {/each}
      </div>
    {/if}

    {#if rtt}
      <div class="mt-5 pt-4 border-t border-slate-800">
        <div class="flex items-center justify-between mb-2.5">
          <span class="text-xs font-mono text-(--slate-500) tracking-wider">物理延迟</span>
          <span class="text-[10px] text-(--slate-600)" title="延迟阈值仍在标定期，读数不参与评分与判定">试运行</span>
        </div>
        <div class="flex flex-wrap gap-x-7 gap-y-2">
          {#if rtt.clientMinMS != null}
            <div title={`请求本站的耗时，多次采样取最小值${rtt.jitterMS != null ? `，抖动 ${ms(rtt.jitterMS)} ms` : ''}`}>
              <span class="text-xs text-(--slate-500)">延迟</span>
              <span class="text-sm text-(--slate-200) tnum ml-1.5">{ms(rtt.clientMinMS)}</span>
              <span class="text-xs text-(--slate-600)">ms</span>
            </div>
          {/if}
          {#if rtt.serverTCPMS != null}
            <div title="本站从探测窗口内 TCP 连接读取的往返时间">
              <span class="text-xs text-(--slate-500)">TCP 延迟</span>
              <span class="text-sm text-(--slate-200) tnum ml-1.5">{ms(rtt.serverTCPMS)}</span>
              <span class="text-xs text-(--slate-600)">ms</span>
            </div>
          {/if}
          {#if rtt.deltaMS != null}
            <div title="同窗口内应用延迟与服务器 TCP 延迟之差，可辅助参考代理中转额外耗时">
              <span class="text-xs text-(--slate-500)">延迟差</span>
              <span class="text-sm text-(--slate-200) tnum ml-1.5">{ms(rtt.deltaMS)}</span>
              <span class="text-xs text-(--slate-600)">ms</span>
            </div>
          {/if}
        </div>
      </div>
    {/if}
    </div>
  {/if}
</div>
