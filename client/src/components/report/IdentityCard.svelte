<script lang="ts">
  import type { IdentityReadout, RTTReadout } from '../../lib/report';
  import { stateVar, stateSoft } from '../../lib/report';

  interface Props {
    identity: IdentityReadout;
    rtt?: RTTReadout;
    isScanning?: boolean;
    revealed?: boolean;
    onToggleReveal?: () => void;
    onCopy?: () => void;
  }
  let { identity, rtt, isScanning = false, revealed = false, onToggleReveal, onCopy }: Props = $props();

  // 仪表读数固定一位小数（tnum 下位数对齐）
  const ms = (x: number) => x.toFixed(1);

  // 国家码 → 国旗 emoji
  function flag(code: string): string {
    if (!code || code.length !== 2) return '';
    const cc = code.toUpperCase();
    if (!/^[A-Z]{2}$/.test(cc)) return '';
    return String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
  }

  // 类型标签图标
  function typeIcon(label: string): string {
    if (label.includes('机房')) return 'fa-server';
    if (label.includes('住宅')) return 'fa-house';
    if (label.includes('移动')) return 'fa-mobile-screen';
    return 'fa-circle-question';
  }

  // 欺诈评分中文档位
  let fraudWord = $derived(
    identity.fraudScore < 25 ? '很低' : identity.fraudScore < 50 ? '较低' : identity.fraudScore < 75 ? '偏高' : '高危',
  );
  let fraudColor = $derived(identity.fraudScore >= 50 ? 'var(--leak)' : identity.fraudScore >= 25 ? 'var(--warn)' : 'var(--sig)');
</script>

<div class="glass-card rounded-xl p-6 reveal">
  <header class="flex items-center justify-between mb-5">
    <div class="flex items-center gap-2.5">
      <i class="fa-solid fa-id-card text-(--radar-400)"></i>
      <h3 class="text-sm font-semibold text-white tracking-wide">网络身份</h3>
    </div>
    <span class="dot" style="background:{stateVar(identity.state)}; --_soft:{stateSoft(identity.state)}"></span>
  </header>

  {#if isScanning}
    <div class="flex flex-col md:flex-row gap-6">
      <div class="md:w-[32%] space-y-3">
        <div class="h-8 w-44 bg-slate-800 rounded animate-pulse"></div>
        <div class="h-6 w-24 bg-slate-800/60 rounded animate-pulse"></div>
      </div>
      <div class="flex-1 grid grid-cols-2 gap-4">
        {#each Array(4) as _}<div class="h-10 bg-slate-800/50 rounded animate-pulse"></div>{/each}
      </div>
    </div>
  {:else}
    <div class="flex flex-col md:flex-row gap-6">
      <!-- 左：核芯 IP -->
      <div class="md:w-[32%] md:border-r border-slate-800 md:pr-6 flex flex-col justify-center">
        <div class="text-xs text-(--slate-500) mb-1.5">出口 IP</div>
        <div class="flex items-center gap-2.5">
          <button
            class="text-2xl font-mono font-semibold text-white hover:text-(--radar-400) transition-colors cursor-pointer"
            onclick={onToggleReveal}
            title={revealed ? '点击脱敏' : '点击显示完整 IP'}
            data-ip-display
          >{revealed ? identity.ip || '—' : identity.masked}</button>
          {#if onCopy}
            <button class="text-(--slate-500) hover:text-(--radar-400) transition-colors cursor-pointer" onclick={onCopy} title="复制完整 IP" aria-label="复制">
              <i class="fa-regular fa-copy text-sm"></i>
            </button>
          {/if}
        </div>
        <div class="mt-3.5">
          <span
            class="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-md"
            style="color:{stateVar(identity.state)}; background:{stateSoft(identity.state)}"
          >
            <i class="fa-solid {typeIcon(identity.usageLabel)}"></i>{identity.usageLabel}
          </span>
        </div>
      </div>

      <!-- 右：2x2 属性栅格 -->
      <div class="flex-1 grid grid-cols-2 gap-x-6 gap-y-5">
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
          <div class="text-sm text-(--slate-200) truncate" title="{identity.org} {identity.asn}">{identity.org}</div>
        </div>

        <div>
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-shield-halved text-[10px]"></i> 欺诈评分
          </div>
          <div class="text-sm">
            <span class="tnum text-(--slate-200)">{identity.fraudScore}</span>
            <span class="text-(--slate-600)"> / 100</span>
            <span class="ml-1" style="color:{fraudColor}">({fraudWord})</span>
          </div>
        </div>

        <div>
          <div class="text-xs text-(--slate-500) mb-1 flex items-center gap-1.5">
            <i class="fa-solid fa-list-check text-[10px]"></i> 黑名单
          </div>
          <div class="text-sm flex items-center gap-1.5">
            <span class="dot" style="background:{identity.blacklist.length ? 'var(--leak)' : 'var(--sig)'}; --_soft:transparent"></span>
            <span style="color:{identity.blacklist.length ? 'var(--leak)' : 'var(--sig)'}">{identity.blacklist.length ? `命中 ${identity.blacklist.length}` : '未命中'}</span>
          </div>
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

    <!-- 物理延迟读数条（Phase 1）：纯测量值，无判定无语义色，标定完成前不亮灯。分享卡以同一形态呈现（shareCard.ts）。 -->
    {#if rtt}
      <div class="mt-5 pt-4 border-t border-slate-800">
        <div class="flex items-center justify-between mb-2.5">
          <span
            class="text-xs font-mono text-(--slate-500) tracking-wider"
            title="任何代理都会在浏览器与服务器之间插入额外物理链路，往返时延藏不住"
          >物理延迟</span>
          <span class="text-[10px] text-(--slate-600)" title="延迟阈值仍在标定期，读数不参与评分与判定">试运行 · 不参与评分</span>
        </div>
        <div class="flex flex-wrap gap-x-7 gap-y-2">
          {#if rtt.clientMinMS != null}
            <div title={rtt.jitterMS != null ? `多次采样取最小值；抖动 ${ms(rtt.jitterMS)} ms` : '多次采样取最小值'}>
              <span class="text-xs text-(--slate-500)">浏览器 ↔ 服务器</span>
              <span class="text-sm text-(--slate-200) tnum ml-1.5">{ms(rtt.clientMinMS)}</span>
              <span class="text-xs text-(--slate-600)">ms</span>
            </div>
          {/if}
          {#if rtt.serverTCPMS != null}
            <div title="服务器侧内核实测：出口 IP 到服务器的 TCP 往返">
              <span class="text-xs text-(--slate-500)">出口 ↔ 服务器</span>
              <span class="text-sm text-(--slate-200) tnum ml-1.5">{ms(rtt.serverTCPMS)}</span>
              <span class="text-xs text-(--slate-600)">ms</span>
            </div>
          {/if}
          {#if rtt.deltaMS != null}
            <div title="两段之差，近似浏览器到出口这一段；含测量噪声，可能略偏">
              <span class="text-xs text-(--slate-500)">Δ 浏览器 ↔ 出口</span>
              <span class="text-sm text-(--slate-200) tnum ml-1.5">{ms(rtt.deltaMS)}</span>
              <span class="text-xs text-(--slate-600)">ms</span>
            </div>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</div>
