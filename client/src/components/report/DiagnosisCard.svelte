<script lang="ts">
  import type { ReportModel, Severity } from '../../lib/report';
  import { stateVar, stateSoft } from '../../lib/report';

  interface Props {
    report: ReportModel | null;
    isScanning?: boolean;
    active?: boolean;
    onCopyPromo?: (code: string) => void;
    onStart?: () => void;
  }
  const FINDING_LINKS: Record<string, string> = {
    'dns-leak': '/dns-leak-test/',
    'webrtc-leak': '/webrtc-leak-test/',
  };

  let { report, isScanning = false, active = false, onCopyPromo, onStart }: Props = $props();

  const SEV: Record<Severity, { color: string; soft: string; label: string; icon: string }> = {
    leak: { color: 'var(--leak)', soft: 'var(--leak-soft)', label: '暴露', icon: 'fa-triangle-exclamation' },
    contradiction: { color: 'var(--warn)', soft: 'var(--warn-soft)', label: '冲突', icon: 'fa-not-equal' },
    caution: { color: 'var(--warn)', soft: 'var(--warn-soft)', label: '留意', icon: 'fa-circle-exclamation' },
    info: { color: 'var(--unknown)', soft: 'var(--unknown-soft)', label: '提示', icon: 'fa-circle-info' },
  };

  // 合作推荐位配置开关（默认关闭，通过 PUBLIC_PARTNER_ENABLED=true 开启）
  const ENV = (import.meta as unknown as { env?: Record<string, string | boolean> }).env ?? {};
  const partnerEnabled = ENV.PUBLIC_PARTNER_ENABLED === true || ENV.PUBLIC_PARTNER_ENABLED === 'true' || ENV.PUBLIC_PARTNER_ENABLED === '1';
  const partnerUrl = typeof ENV.PUBLIC_PARTNER_URL === 'string' && ENV.PUBLIC_PARTNER_URL ? ENV.PUBLIC_PARTNER_URL : 'https://proxy-seller.com/';
  const partnerCode = typeof ENV.PUBLIC_PARTNER_CODE === 'string' && ENV.PUBLIC_PARTNER_CODE ? ENV.PUBLIC_PARTNER_CODE : 'RADAR';
</script>

<div class="glass-card rounded-xl p-6 h-full flex flex-col reveal" class:scan-focus={active}>
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
        <p class="text-xl md:text-2xl font-bold text-white leading-snug">{report.verdict.headline}</p>
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
            {@const href = FINDING_LINKS[f.id]}
            <li class="flex items-start gap-2.5">
              <span class="text-[10px] font-mono px-1.5 py-1 rounded shrink-0 flex items-center gap-1" style="color:{m.color}; background:{m.soft}">
                <i class="fa-solid {m.icon}"></i>{m.label}
              </span>
              <div class="min-w-0 flex-1">
                {#if href}
                  <a {href} class="text-sm text-(--slate-200) leading-snug hover:text-(--radar-400) underline decoration-slate-600 underline-offset-4">
                    {f.title}<i class="fa-solid fa-arrow-right ml-1.5 text-[10px]" aria-hidden="true"></i>
                  </a>
                {:else}
                  <span class="text-sm text-(--slate-200) leading-snug">{f.title}</span>
                {/if}
                <!-- 窄屏放开换行：单行省略号会把暴露项的证据（被比对的两个值）截掉一半 -->
                <span class="text-xs text-(--slate-500) block sm:truncate mt-0.5">{f.fact}</span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    {#if partnerEnabled}
      <!-- 合作推荐位：完全复用上方暴露项的「左胶囊 + 右主副双行」组件语法与像素对齐 -->
      <div class="mt-5 pt-4 border-t border-slate-800 flex items-start gap-2.5">
        <span
          class="text-[10px] font-mono px-1.5 py-1 rounded shrink-0 flex items-center gap-1"
          style="color:#38bdf8; background:rgba(56, 189, 248, 0.12)"
        >
          <span class="w-[1.25em] text-center leading-none">✦</span>推荐
        </span>

        <div class="min-w-0 flex-1">
          <span class="text-sm text-(--slate-200) leading-snug block">
            需要纯净的原生住宅或 ISP 代理？
          </span>
          <div class="mt-1 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs text-(--slate-500)">
            <a
              href={partnerUrl}
              target="_blank"
              rel="noopener sponsored"
              class="text-(--slate-300) hover:text-white underline decoration-slate-600 hover:decoration-slate-300 underline-offset-4 transition-colors"
            >
              前往 Proxy-Seller 获取 ↗
            </a>
            {#if partnerCode}
              <span class="text-(--slate-600)">·</span>
              <button
                type="button"
                onclick={() => onCopyPromo?.(partnerCode)}
                class="inline-flex items-center gap-1.5 text-(--slate-400) hover:text-(--slate-200) transition-colors cursor-pointer"
                title="结账时输入可享专属折扣，点击复制"
              >
                <span>优惠码</span>
                <code class="px-1.5 py-0.5 rounded bg-white/5 text-(--slate-300) font-mono text-[11px]">{partnerCode}</code>
                <i class="fa-regular fa-copy text-[11px]"></i>
              </button>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  {:else}
    <div class="flex flex-col items-start justify-center h-full gap-2.5">
      <p class="text-lg font-semibold text-white">待检测</p>
      <p class="text-sm text-(--slate-400) leading-relaxed max-w-xl">
        点击开始检测当前出口 IP、浏览器时区与语言一致性、DNS / WebRTC / IPv6 泄露及环境指纹。
      </p>
      {#if onStart}
        <button
          type="button"
          onclick={onStart}
          class="mt-1 text-sm px-4 py-2 rounded-lg transition-colors cursor-pointer"
          style="color:var(--radar-400); background:var(--sig-soft)"
        >开始检测</button>
      {/if}
    </div>
  {/if}
</div>
