<script lang="ts">
  import { getMessages, localePath, type Locale } from '../../i18n';
  import type { ReportModel, Severity } from '../../lib/report';
  import { stateVar, stateSoft } from '../../lib/report';
  import { specialistToolFromHref, trackEvent } from '../../lib/analytics';

  interface Props {
    locale?: Locale;
    report: ReportModel | null;
    isScanning?: boolean;
    active?: boolean;
    onCopyPromo?: (code: string) => void;
    onStart?: () => void;
  }
  const FINDING_LINKS: Record<string, string> = {
    'dns-leak': '/dns-leak-test/',
    'webrtc-leak': '/webrtc-leak-test/',
    'ipv6-leak': '/ipv6-leak-test/',
  };

  let { locale = 'zh-CN', report, isScanning = false, active = false, onCopyPromo, onStart }: Props = $props();

  let msg = $derived(getMessages(locale));
  let d = $derived(msg.home.diagnosis);

  let sevMap = $derived<Record<Severity, { color: string; soft: string; label: string; icon: string }>>({
    leak: { color: 'var(--leak)', soft: 'var(--leak-soft)', label: d.sev.leak, icon: 'fa-triangle-exclamation' },
    contradiction: { color: 'var(--warn)', soft: 'var(--warn-soft)', label: d.sev.contradiction, icon: 'fa-not-equal' },
    caution: { color: 'var(--warn)', soft: 'var(--warn-soft)', label: d.sev.caution, icon: 'fa-circle-exclamation' },
    info: { color: 'var(--unknown)', soft: 'var(--unknown-soft)', label: d.sev.info, icon: 'fa-circle-info' },
  });

  const ENV = (import.meta as unknown as { env?: Record<string, string | boolean> }).env ?? {};
  const partnerEnabled = ENV.PUBLIC_PARTNER_ENABLED === true || ENV.PUBLIC_PARTNER_ENABLED === 'true' || ENV.PUBLIC_PARTNER_ENABLED === '1';
  const partnerUrl = typeof ENV.PUBLIC_PARTNER_URL === 'string' && ENV.PUBLIC_PARTNER_URL ? ENV.PUBLIC_PARTNER_URL : 'https://proxy-seller.com/';
  const partnerCode = typeof ENV.PUBLIC_PARTNER_CODE === 'string' && ENV.PUBLIC_PARTNER_CODE ? ENV.PUBLIC_PARTNER_CODE : 'RADAR';
</script>

<div class="glass-card rounded-xl p-6 h-full flex flex-col" class:scan-focus={active}>
  {#if isScanning}
    <div class="space-y-3">
      <div class="h-6 w-2/5 bg-slate-800 rounded animate-pulse"></div>
      <div class="h-4 w-4/5 bg-slate-800/70 rounded animate-pulse"></div>
      <div class="h-4 w-3/5 bg-slate-800/50 rounded animate-pulse mt-6"></div>
    </div>
  {:else if report}
    <div class="flex flex-col flex-1 scan-settle">
      <div class="flex items-start gap-3">
        <span class="dot mt-2" style="background:{stateVar(report.verdict.state)}; --_soft:{stateSoft(report.verdict.state)}"></span>
        <div>
          <p class="text-xl md:text-2xl font-bold text-white leading-snug">{report.verdict.headline}</p>
          <p class="text-sm text-(--slate-400) mt-1.5 leading-relaxed max-w-xl">{report.verdict.sub}</p>
        </div>
      </div>

      <div class="mt-5 pt-4 border-t border-slate-800 flex-1">
        {#if report.findings.length === 0}
          <div class="flex items-center gap-2 text-sm text-(--slate-300)">
            <i class="fa-solid fa-circle-check" style="color:var(--sig)"></i> {d.noIssues}
          </div>
        {:else}
          <div class="flex items-center justify-between mb-3">
            <span class="text-xs font-mono text-(--slate-500) tracking-wider">{d.findingsLabel}</span>
            <span class="text-xs font-mono text-(--slate-600)">{report.findings.length}</span>
          </div>
          <ul class="space-y-2.5">
            {#each report.findings as f}
              {@const m = sevMap[f.severity]}
              {@const baseHref = FINDING_LINKS[f.id]}
              {@const href = baseHref ? localePath(baseHref, locale) : undefined}
              <li class="flex items-start gap-2.5">
                <span class="text-[10px] font-mono px-1.5 py-1 rounded shrink-0 flex items-center gap-1" style="color:{m.color}; background:{m.soft}">
                  <i class="fa-solid {m.icon}"></i>{m.label}
                </span>
                <div class="min-w-0 flex-1">
                  {#if href}
                    <a
                      {href}
                      onclick={() => {
                        const to = specialistToolFromHref(href);
                        if (to) trackEvent('tool-open', { from: 'home', to, source: 'finding' });
                      }}
                      class="text-sm text-(--slate-200) leading-snug hover:text-(--radar-400) underline decoration-slate-600 underline-offset-4 wrap-anywhere"
                    >
                      {f.title}<i class="fa-solid fa-arrow-right ml-1.5 text-[10px]" aria-hidden="true"></i>
                    </a>
                  {:else}
                    <span class="text-sm text-(--slate-200) leading-snug wrap-anywhere">{f.title}</span>
                  {/if}
                  <span class="text-xs text-(--slate-400) block mt-0.5 leading-relaxed wrap-anywhere">{f.fact}</span>
                  {#if f.fix && f.severity !== 'info'}
                    <span class="text-xs text-(--slate-500) block mt-0.5 leading-relaxed wrap-anywhere">{d.fixPrefix}{f.fix}</span>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      {#if partnerEnabled}
        <div class="mt-5 pt-4 border-t border-slate-800 flex items-start gap-2.5">
          <span
            class="text-[10px] font-mono px-1.5 py-1 rounded shrink-0 flex items-center gap-1"
            style="color:#38bdf8; background:rgba(56, 189, 248, 0.12)"
          >
            <span class="w-[1.25em] text-center leading-none">✦</span>{d.partnerBadge}
          </span>

          <div class="min-w-0 flex-1">
            <span class="text-sm text-(--slate-200) leading-snug block">
              {d.partnerTitle}
            </span>
            <div class="mt-1 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs text-(--slate-500)">
              <a
                href={partnerUrl}
                target="_blank"
                rel="noopener sponsored"
                class="text-(--slate-300) hover:text-white underline decoration-slate-600 hover:decoration-slate-300 underline-offset-4 transition-colors"
              >
                {d.partnerCta}
              </a>
              {#if partnerCode}
                <span class="text-(--slate-600)">·</span>
                <button
                  type="button"
                  onclick={() => onCopyPromo?.(partnerCode)}
                  class="inline-flex items-center gap-1.5 text-(--slate-400) hover:text-(--slate-200) transition-colors cursor-pointer"
                  title={d.partnerCodeTitle}
                >
                  <span>{d.partnerCodeLabel}</span>
                  <code class="px-1.5 py-0.5 rounded bg-white/5 text-(--slate-300) font-mono text-[11px]">{partnerCode}</code>
                  <i class="fa-regular fa-copy text-[11px]"></i>
                </button>
              {/if}
            </div>
          </div>
        </div>
      {/if}
    </div>
  {:else}
    <div class="flex flex-col items-start justify-center h-full gap-2.5">
      <p class="text-lg font-semibold text-white">{msg.common.idle}</p>
      <p class="text-sm text-(--slate-400) leading-relaxed max-w-xl">
        {d.idleDesc}
      </p>
      {#if onStart}
        <button
          type="button"
          onclick={onStart}
          class="mt-1 text-sm px-4 py-2 rounded-lg transition-colors cursor-pointer"
          style="color:var(--radar-400); background:var(--sig-soft)"
        >{msg.common.startScan}</button>
      {/if}
    </div>
  {/if}
</div>
