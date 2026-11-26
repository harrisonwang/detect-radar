<script lang="ts">
  import { onMount } from 'svelte';
  import { getMessages, localePath, type Locale } from '../i18n';
  import AppHeader from './report/AppHeader.svelte';
  import ExitIPAddress from './report/ExitIPAddress.svelte';
  import ReportToast from './report/ReportToast.svelte';
  import IPv6FlowStage from './report/IPv6FlowStage.svelte';
  import { dnsExitType, dnsProviderName, exitTypeLabel } from '../lib/dnsPresentation';
  import {
    compareIPv6,
    getIPv6Status,
    ipv6History,
    ipv6VerdictNote,
    measureIPv6,
    stackRegionLabel,
    type IPv6Check,
    type IPv6History,
    type IPv6Progress,
    type StackExit,
  } from '../lib/ipv6Check';
  import { emptyHistory } from '../lib/recentHistory';
  import { trackEvent } from '../lib/analytics';
  import { isAutomatedCrawler } from '../lib/crawler';
  import { renderIPv6ShareCard, saveShareImage } from '../lib/toolShareCard';
  import { copyShareText, formatIPv6ShareText } from '../lib/shareSummary';

  interface Props {
    locale?: Locale;
  }
  let { locale = 'zh-CN' }: Props = $props();

  let m = $derived(getMessages(locale));
  let statusMap = $derived(getIPv6Status(locale));

  let history = $state.raw<IPv6History>(emptyHistory<IPv6Check>());
  let ready = $state(false);
  let busy = $state(false);
  let liveProgress = $state.raw<IPv6Progress | null>(null);
  let revealExit = $state(true);
  let capturing = $state(false);
  let toast: ReturnType<typeof ReportToast> | undefined = $state();

  let current = $derived(history.current);
  let comparison = $derived(
    current && history.previous ? compareIPv6(history.previous, current, locale) : null,
  );
  let verdict = $derived(current?.verdict);
  let verdictNote = $derived(current && verdict ? ipv6VerdictNote(verdict, current.v4, current.v6, locale) : '');
  let status = $derived(verdict ? statusMap[verdict.state] : statusMap.unknown);

  onMount(() => {
    if (isAutomatedCrawler()) {
      ready = true;
      return;
    }
    async function restore() {
      try {
        history = await ipv6History();
      } catch {
      }
      ready = true;
      const navigation = performance.getEntriesByType('navigation')[0] as
        | PerformanceNavigationTiming
        | undefined;
      if (!history.current || navigation?.type === 'navigate') await run('auto');
    }
    void restore();
  });

  async function run(mode: 'auto' | 'manual' | 'retest' = 'retest') {
    if (busy || !ready) return;
    if (mode === 'retest') trackEvent('scan-retest', { page: 'ipv6' });
    else trackEvent('scan-start', { page: 'ipv6', trigger: mode });
    busy = true;
    liveProgress = null;
    try {
      const result = await measureIPv6(
        (progress) => {
          liveProgress = progress;
        },
        {},
        locale,
      );
      try {
        history = await ipv6History(result, history.current);
      } catch {
        history = { previous: history.current, current: result };
      }
    } finally {
      liveProgress = null;
      busy = false;
    }
  }

  async function handleScreenshot() {
    if (!current || busy || capturing || !verdict) return;
    capturing = true;
    try {
      const blob = await renderIPv6ShareCard({
        time: current.time,
        verdictState: verdict.state,
        verdictTitle: status.title,
        verdictReason: verdict.reason,
        verdictNote,
        v4: current.v4,
        v4NetworkName: current.v4?.ip
          ? (dnsProviderName(current.v4.network, locale) || m.common.unknownNetwork)
          : m.ipv6.page.unreachableShort,
        v4Region: current.v4?.ip ? stackRegionLabel(current.v4, locale) : '',
        v4TypeLabel: current.v4?.ip ? exitTypeLabel(dnsExitType(current.v4), locale) : '',
        v6: current.v6,
        v6NetworkName: current.v6?.ip
          ? (dnsProviderName(current.v6.network, locale) || m.common.unknownNetwork)
          : m.ipv6.page.unreachableShort,
        v6Region: current.v6?.ip ? stackRegionLabel(current.v6, locale) : '',
        v6TypeLabel: current.v6?.ip ? exitTypeLabel(dnsExitType(current.v6), locale) : '',
        locale,
      });
      const shareMsg = await saveShareImage(blob, 'detectradar-ipv6', locale);
      trackEvent('share-save-image', { page: 'ipv6' });
      toast?.show(shareMsg);
    } catch (err) {
      toast?.show(m.common.imageFailed((err as Error).message));
    } finally {
      capturing = false;
    }
  }

  async function handleCopyText() {
    if (!current || busy || !verdict) return;
    trackEvent('share-copy-text', { page: 'ipv6' });
    const text = formatIPv6ShareText(
      {
        verdictState: verdict.state,
        verdictTitle: status.title,
        verdictReason: verdict.reason,
        verdictNote,
        v4: current.v4,
        v4NetworkName: current.v4?.ip
          ? (dnsProviderName(current.v4.network, locale) || m.common.unknownNetwork)
          : m.ipv6.page.unreachableShort,
        v4Region: current.v4?.ip ? stackRegionLabel(current.v4, locale) : '',
        v4TypeLabel: current.v4?.ip ? exitTypeLabel(dnsExitType(current.v4), locale) : '',
        v6: current.v6,
        v6NetworkName: current.v6?.ip
          ? (dnsProviderName(current.v6.network, locale) || m.common.unknownNetwork)
          : m.ipv6.page.unreachableShort,
        v6Region: current.v6?.ip ? stackRegionLabel(current.v6, locale) : '',
        v6TypeLabel: current.v6?.ip ? exitTypeLabel(dnsExitType(current.v6), locale) : '',
        comparisonSummary: comparison?.summary,
      },
      locale,
    );
    toast?.show(await copyShareText(text, locale));
  }
</script>

{#snippet stackReadout(
  exit: StackExit | null,
  family: 'v4' | 'v6',
  pending: boolean,
  waiting: boolean,
  errKind?: 'limited' | 'unreachable',
  isLeak = false,
  isSplit = false,
)}
  <div class="reading">
    <p class="field" title={family === 'v4' ? m.ipv6.page.v4Tooltip : m.ipv6.page.v6Tooltip}>
      <i class="fa-solid {family === 'v4' ? 'fa-globe' : 'fa-network-wired'}" aria-hidden="true"></i>
      {family === 'v4' ? m.ipv6.page.v4Label : m.ipv6.page.v6Label}
    </p>
    {#if exit?.ip}
      {@const networkType = exitTypeLabel(dnsExitType(exit), locale)}
      <div class="network-identity exit-identity">
        <span class="network-name" class:leak-text={isLeak}>{dnsProviderName(exit.network, locale) || m.common.unknownNetwork}</span>
        <span class="region">{stackRegionLabel(exit, locale)}</span>
      </div>
      <div class="mt-0.5 mb-1.5">
        <ExitIPAddress
          {locale}
          ip={exit.ip}
          revealed={revealExit}
          onToggleReveal={() => {
            revealExit = !revealExit;
          }}
          onCopyFeedback={(message) => toast?.show(message)}
        />
      </div>
      <div class="exit-meta">
        {#if isLeak}
          <span class="badge danger">{m.ipv6.page.badgeLeak}</span>
        {:else if isSplit}
          <span class="badge">{m.ipv6.page.badgeSplit}</span>
        {/if}
        {#if networkType}<span class="badge">{networkType}</span>{/if}
        {#if exit.asn}<span class="font-mono">{exit.asn}</span>{/if}
        {#if typeof exit.latencyMs === 'number'}<span class="font-mono">{exit.latencyMs} ms</span>{/if}
      </div>
    {:else if pending}
      <div class="mt-3 h-5 w-40 rounded bg-white/8" aria-hidden="true"></div>
      <div class="mt-2 h-3 w-24 rounded bg-white/5" aria-hidden="true"></div>
    {:else if waiting}
      <p class="readout-placeholder">{m.common.idle}</p>
    {:else if errKind === 'limited'}
      <p class="readout-placeholder">{m.ipv6.page.errLimited}</p>
    {:else if family === 'v6'}
      <p class="readout-placeholder">{m.ipv6.page.v6Unreachable}</p>
    {:else}
      <p class="readout-placeholder">{m.ipv6.page.v4Unreachable}</p>
    {/if}
  </div>
{/snippet}

<AppHeader
  page="ipv6"
  {locale}
  onCopyText={handleCopyText}
  onScreenshot={handleScreenshot}
  onRescan={() => run('retest')}
  isScanning={!ready || busy}
  ready={!!current}
/>

<main class="report-main flex-1 min-w-0">
  <header class="mb-6">
    <h1 class="report-title"><span class="text-radar">{m.ipv6.titleAccent}</span>{m.ipv6.titleRest}</h1>
  </header>

  <div role="status" class="sr-only" data-nosnippet>
    {busy
      ? current
        ? m.ipv6.page.srRetesting
        : m.ipv6.page.srTesting
      : current
        ? status.title
        : ''}
  </div>

  <div class="diagnostic-layout">
    {#if current}
      <section class="glass-card result-card" aria-label={m.ipv6.page.resultAria} data-nosnippet>
        {#if current.v4Error === 'limited' || current.v6Error === 'limited'}
          <div class="result-notice">
            <i class="fa-solid fa-circle-info" aria-hidden="true"></i>
            <span>{m.ipv6.page.noticeLimited}</span>
          </div>
        {/if}

        <IPv6FlowStage
          {locale}
          v4={current.v4}
          v6={current.v6}
          verdictState={verdict?.state ?? 'unknown'}
          {verdictNote}
          {revealExit}
          {busy}
          {liveProgress}
          hasResult={true}
        />

        {#if busy}
          <div class="progress-track" role="progressbar" aria-label={m.ipv6.page.retestProgressAria}>
            <span></span>
          </div>
        {/if}

        <header
          class="diagnosis"
          style:--verdict-color={status.color}
          style:--verdict-soft={status.soft}
        >
          <p class="card-label">{busy ? m.common.retestingPrevious : m.common.thisScan}</p>
          <p class="verdict-title">
            <span class="dot" class:loading-dot={busy} aria-hidden="true"></span>{status.title}
          </p>
          {#if verdictNote}
            <p class="diagnosis-note">{verdictNote}</p>
          {/if}
        </header>

        <div class="columns" class:retesting={busy}>
          {@render stackReadout(current.v4, 'v4', false, false, current.v4Error)}
          {@render stackReadout(
            current.v6,
            'v6',
            false,
            false,
            current.v6Error,
            verdict?.state === 'leaked',
            verdict?.reason === 'overseas-split',
          )}
        </div>
      </section>
    {:else}
      {@const isWaiting = ready && !busy}
      <section class="glass-card result-card" aria-label={isWaiting ? m.ipv6.page.waitingTitle : m.ipv6.page.testingTitle} data-nosnippet>
        <IPv6FlowStage
          {locale}
          v4={liveProgress?.v4 ?? null}
          v6={liveProgress?.v6 ?? null}
          verdictState="unknown"
          {revealExit}
          busy={!isWaiting}
          {liveProgress}
          hasResult={false}
        />
        {#if !isWaiting}
          <div class="progress-track" role="progressbar" aria-label={m.ipv6.page.progressAria}>
            <span></span>
          </div>
        {/if}
        <header class="diagnosis">
          {#if isWaiting}
            <p class="verdict-title">
              <span class="dot" aria-hidden="true"></span>{m.ipv6.page.waitingTitle}
            </p>
            <p class="diagnosis-note">{m.ipv6.page.waitingDesc}</p>
            <button
              type="button"
              onclick={() => run('manual')}
              class="mt-3 text-sm px-3.5 py-1.5 rounded-lg transition-colors cursor-pointer"
              style="color:var(--radar-400); background:var(--sig-soft)"
            >{m.common.startScan}</button>
          {:else}
            <p class="verdict-title">
              <span class="dot loading-dot" aria-hidden="true"></span>{m.ipv6.page.testingTitle}
            </p>
            <p class="diagnosis-note">{m.ipv6.page.testingDesc}</p>
          {/if}
        </header>
        <div class="columns">
          {@render stackReadout(liveProgress?.v4 ?? null, 'v4', !isWaiting && !liveProgress?.v4Done, isWaiting)}
          {@render stackReadout(liveProgress?.v6 ?? null, 'v6', !isWaiting && !liveProgress?.v6Done, isWaiting)}
        </div>
      </section>
    {/if}

    <div class="side-stack">
      {#if comparison}
        <aside class="glass-card side-card" aria-labelledby="ipv6-comparison-title" data-nosnippet>
          <p class="side-heading" id="ipv6-comparison-title">
            <i class="fa-solid fa-arrow-right-arrow-left" aria-hidden="true"></i>
            {busy ? m.common.lastComparison : m.common.vsLast}
          </p>
          {#if comparison.summary}
            <p class="comparison-summary">{comparison.summary}</p>
          {:else}
            {#if comparison.previousVerdict}
              <p class="verdict-change" style:--change-color={status.color}>
                <span class="sr-only">{m.common.previousScanSr}</span>
                {#if comparison.improved}
                  <s>{statusMap[comparison.previousVerdict].title}</s>
                {:else}
                  <span>{statusMap[comparison.previousVerdict].title}</span>
                {/if}
                <i class="fa-solid fa-arrow-right" aria-hidden="true"></i>
                <span class="sr-only">{m.common.changedToSr}</span>
                <strong>{status.title}</strong>
              </p>
            {/if}
            <dl class="change-list">
              <div>
                <dt>{m.ipv6.page.v4Label}</dt>
                <dd>{comparison.v4Change}</dd>
              </div>
              <div>
                <dt>{m.ipv6.page.v6Label}</dt>
                <dd>{comparison.v6Change}</dd>
              </div>
            </dl>
          {/if}
        </aside>
      {/if}

      <aside class="glass-card side-card" aria-labelledby="ipv6-info-title">
        <p class="side-heading" id="ipv6-info-title">
          <i class="fa-solid fa-circle-info" aria-hidden="true"></i>{m.common.howItWorks}
        </p>
        <ul class="rule-note">
          <li>{m.ipv6.page.rules[0]}</li>
          <li>{m.ipv6.page.rules[1]}<strong>{statusMap.leaked.title}</strong></li>
          <li>
            {m.ipv6.page.rules[2]}<strong>{statusMap.clear.title}</strong>{locale === 'en' ? ' — see ' : '，完整口径见 '}<a href={localePath('/about/#ipv6', locale)} class="text-(--radar-400) underline underline-offset-4">{m.common.fullCriteriaLink}</a>
          </li>
        </ul>
      </aside>
    </div>
  </div>
</main>

<ReportToast bind:this={toast} />

<style>
  .result-card {
    border-radius: 0.875rem;
    overflow: hidden;
  }
  .diagnosis {
    padding: 1.125rem 1.25rem 1.25rem;
    border-bottom: 1px solid var(--line);
    background: linear-gradient(110deg, var(--verdict-soft, var(--sig-soft)), transparent 66%);
  }
  .card-label,
  .side-heading {
    color: var(--slate-400);
    font-size: 0.8125rem;
    font-weight: 500;
  }
  .card-label {
    margin-bottom: 0.5rem;
  }
  .diagnostic-layout {
    display: grid;
    gap: 1rem;
    align-items: start;
  }
  .diagnostic-layout > * {
    min-width: 0;
  }
  .side-stack {
    display: grid;
    gap: 1rem;
  }
  .side-card {
    border-radius: 0.875rem;
    padding: 1.125rem 1.25rem;
  }
  .side-heading {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .side-heading i {
    font-size: 0.75rem;
  }
  .comparison-summary {
    margin-top: 0.825rem;
    font-weight: 600;
    color: var(--slate-200);
  }
  .rule-note {
    margin-top: 0.7rem;
    color: var(--slate-300);
    font-size: 0.875rem;
    line-height: 1.65;
    list-style: disc;
    padding-inline-start: 1rem;
  }
  .rule-note li + li {
    margin-top: 0.375rem;
  }
  .rule-note strong {
    color: var(--slate-200);
    font-weight: 600;
  }
  .change-list {
    margin-top: 0.875rem;
    display: grid;
    gap: 0.5rem;
    font-size: 0.875rem;
  }
  .verdict-change + .change-list {
    border-top: 1px solid var(--line);
    padding-top: 0.875rem;
  }
  .change-list > div {
    display: grid;
    grid-template-columns: minmax(4.75rem, auto) minmax(0, 1fr);
    align-items: baseline;
    gap: 0.5rem;
  }
  .change-list dt {
    color: var(--slate-400);
    font-size: 0.8125rem;
    white-space: nowrap;
  }
  .change-list dd {
    color: var(--slate-200);
    overflow-wrap: anywhere;
  }
  .change-list s {
    color: var(--slate-400);
  }
  .verdict-title {
    position: relative;
    min-width: 0;
    color: white;
    font-size: 1.125rem;
    font-weight: 700;
    line-height: 1.4;
  }
  .diagnosis .verdict-title {
    padding-inline-start: 0.975rem;
  }
  .verdict-title .dot {
    position: absolute;
    inset-inline-start: 0;
    top: 0.47em;
    background: var(--verdict-color, var(--sig));
    --_soft: var(--verdict-soft, var(--sig-soft));
  }
  .verdict-change {
    margin-top: 0.75rem;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.55rem;
    color: var(--slate-400);
    font-size: 0.875rem;
  }
  .verdict-change i {
    font-size: 0.7rem;
  }
  .verdict-change strong {
    color: var(--change-color);
    font-weight: 600;
    font-size: 1.125rem;
  }
  .diagnosis-note {
    margin-top: 0.45rem;
    color: var(--slate-300);
    font-size: 0.875rem;
    line-height: 1.55;
    text-wrap: pretty;
  }
  .result-notice {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    padding: 0.55rem 1.25rem;
    background: var(--warn-soft);
    border-bottom: 1px solid var(--line);
    color: var(--slate-200);
    font-size: 0.8125rem;
  }
  .result-notice i {
    color: var(--warn);
    flex: none;
  }
  .columns {
    display: grid;
    transition: opacity 0.2s ease;
  }
  .columns.retesting {
    opacity: 0.55;
  }
  .columns > :global(.reading) + :global(.reading) {
    border-top: 1px solid var(--line);
  }
  .reading {
    min-width: 0;
    padding: 1rem 1.25rem;
  }
  .field {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--slate-400);
    font-size: 0.8125rem;
    font-weight: 500;
  }
  .field i {
    font-size: 0.75rem;
  }
  .network-identity {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.12rem 0.5rem;
  }
  .network-name {
    min-width: 0;
    color: var(--slate-200);
    font-size: 1rem;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .network-name.leak-text {
    color: var(--leak);
  }
  .region {
    color: var(--slate-400);
    font-size: 0.8125rem;
  }
  .exit-identity,
  .readout-placeholder {
    margin-top: 0.625rem;
  }
  .readout-placeholder {
    color: var(--slate-400);
    font-size: 0.9375rem;
  }
  .exit-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.375rem 0.45rem;
    color: var(--slate-400);
    font-size: 0.75rem;
  }
  .badge {
    display: inline-block;
    padding: 0.02rem 0.4rem;
    border: 1px solid var(--line);
    border-radius: 0.3rem;
    color: var(--slate-300);
    font-size: 0.6875rem;
    line-height: 1.5;
    white-space: nowrap;
  }
  .badge.danger {
    color: var(--leak);
    background: var(--leak-soft);
    border-color: transparent;
  }
  .progress-track {
    height: 0.1875rem;
    background: var(--line);
    overflow: hidden;
  }
  .progress-track > span {
    display: block;
    width: 30%;
    height: 100%;
    border-radius: 999px;
    background: var(--radar-400);
    opacity: 0.6;
  }
  @media (min-width: 40rem) {
    .diagnosis,
    .reading {
      padding-inline: 1.5rem;
    }
    .side-card {
      padding: 1.25rem 1.5rem;
    }
    .verdict-title {
      font-size: 1.375rem;
    }
    .columns {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    }
    .columns > :global(.reading) + :global(.reading) {
      border-top: 0;
      border-left: 1px solid var(--line);
    }
  }
  @media (min-width: 64rem) {
    .diagnostic-layout {
      grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
      gap: 0 1.5rem;
    }
  }
  @media (prefers-reduced-motion: no-preference) {
    .progress-track > span {
      animation: sampling 1.7s ease-in-out infinite alternate;
    }
    @keyframes sampling {
      from {
        transform: translateX(-60%);
      }
      to {
        transform: translateX(220%);
      }
    }
  }
</style>
