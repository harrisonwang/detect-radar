<script lang="ts">
  import { onMount } from 'svelte';
  import { getMessages, joinList, localePath, type Locale } from '../i18n';
  import AppHeader from './report/AppHeader.svelte';
  import DNSFlowStage from './report/DNSFlowStage.svelte';
  import ExitIPAddress from './report/ExitIPAddress.svelte';
  import ReportToast from './report/ReportToast.svelte';
  import { compareDNS, DNSCheckError, dnsNetworks, dnsProviderGroups, measureDNS, withDNSAssessment, type DNSProgress } from '../lib/dnsCheck';
  import { trackEvent } from '../lib/analytics';
  import { isAutomatedCrawler } from '../lib/crawler';
  import { dnsHistory, emptyDNSHistory, type DNSHistory } from '../lib/dnsHistory';
  import { dnsExitType, dnsProvider, dnsRegion, dnsVerdictNote, exitTypeLabel, getDnsStatus } from '../lib/dnsPresentation';
  import { renderDNSShareCard, saveShareImage } from '../lib/toolShareCard';
  import { copyShareText, formatDNSShareText } from '../lib/shareSummary';
  import type { DNSExit } from '../lib/types';

  interface Props {
    locale?: Locale;
  }
  let { locale = 'zh-CN' }: Props = $props();

  let m = $derived(getMessages(locale));
  let statusMap = $derived(getDnsStatus(locale));
  let failures = $derived({
    empty: { title: m.dns.failures.emptyTitle, note: m.dns.failures.emptyNote },
    limited: { title: m.dns.failures.failedTitle, note: m.dns.failures.limitedNote },
    unavailable: { title: m.dns.failures.failedTitle, note: m.dns.failures.unavailableNote },
  });

  let history = $state.raw<DNSHistory>(emptyDNSHistory);
  let ready = $state(false);
  let busy = $state(false);
  let failure = $state<'empty' | 'limited' | 'unavailable' | null>(null);
  let pendingExit = $state.raw<DNSExit | null>(null);
  let liveProgress = $state.raw<DNSProgress | null>(null);
  let revealExit = $state(true);
  let capturing = $state(false);
  let toast: ReturnType<typeof ReportToast> | undefined = $state();

  let current = $derived(history.current);
  let networks = $derived(current ? dnsNetworks(current) : null);
  let providers = $derived(networks ? dnsProviderGroups(networks.values(), locale) : []);
  let comparison = $derived(
    current && history.previous && networks
      ? compareDNS(history.previous, current, networks, busy ? 'previous' : 'current', locale)
      : null,
  );
  let verdictState = $derived(current?.verdict?.state ?? 'unknown');
  let status = $derived(statusMap[verdictState]);
  let publicDNSAbroad = $derived(
    verdictState === 'clear' &&
      current?.exit?.country &&
      providers.length > 0 &&
      providers.every((p) => p.type === 'public') &&
      providers.some((p) => p.regions.some((r) => r.country && r.country !== current?.exit?.country)),
  );
  let error = $derived(failure ? failures[failure] : null);
  let note = $derived(
    publicDNSAbroad
      ? m.dns.publicDNSAbroadNote
      : dnsVerdictNote(current?.verdict, busy || error ? 'previous' : 'current', locale),
  );
  let flowProviders = $derived(
    providers.map((p) => ({
      name: p.name,
      country: p.regions[0]?.country ?? '',
      type: p.type,
      suspect: p.suspect,
      count: p.regions.reduce((sum, r) => sum + r.samples.length, 0),
    })),
  );

  onMount(() => {
    if (isAutomatedCrawler()) {
      ready = true;
      return;
    }
    async function restore() {
      try {
        const stored = await dnsHistory();
        const [cur, prev] = await Promise.all([
          stored.current ? withDNSAssessment(stored.current, locale) : null,
          stored.previous ? withDNSAssessment(stored.previous, locale) : null,
        ]);
        history = { current: cur, previous: prev };
      } catch {  }
      ready = true;
      const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
      if (!history.current || navigation?.type === 'navigate') await run('auto');
    }
    void restore();
  });

  async function run(mode: 'auto' | 'manual' | 'retest' = 'retest') {
    if (busy || !ready) return;
    if (mode === 'retest') trackEvent('scan-retest', { page: 'dns' });
    else trackEvent('scan-start', { page: 'dns', trigger: mode });
    busy = true;
    failure = null;
    pendingExit = null;
    liveProgress = null;
    try {
      const result = await measureDNS(
        (exit) => { pendingExit = exit; },
        (progress) => { liveProgress = progress; },
        locale,
      );
      try {
        history = await dnsHistory(result, history.current);
      } catch {
        history = { previous: history.current, current: result };
      }
    } catch (cause) {
      failure = cause instanceof DNSCheckError ? cause.kind : 'unavailable';
    } finally {
      liveProgress = null;
      busy = false;
    }
  }

  async function handleScreenshot() {
    if (!current || busy || capturing) return;
    capturing = true;
    try {
      const blob = await renderDNSShareCard({
        time: current.time,
        verdictState,
        verdictTitle: status.title,
        note,
        exit: current.exit,
        exitNetworkName: current.exit?.ip
          ? (dnsProvider(current.exit.network) || m.common.unknownNetwork)
          : m.common.unavailable,
        exitRegion: current.exit?.ip ? dnsRegion(current.exit.country, locale) : '',
        exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit), locale) : '',
        providers: providers.map((p) => ({
          name: p.name,
          regions: joinList(p.regions.map((r) => dnsRegion(r.country, locale)), locale),
          type: p.type,
          suspect: p.suspect,
        })),
        resolverCount: current.resolvers.length,
        locale,
      });
      const shareMsg = await saveShareImage(blob, 'detectradar-dns', locale);
      trackEvent('share-save-image', { page: 'dns' });
      toast?.show(shareMsg);
    } catch (err) {
      toast?.show(m.common.imageFailed((err as Error).message), 'error');
    } finally {
      capturing = false;
    }
  }

  async function handleCopyText() {
    if (!current || busy) return;
    trackEvent('share-copy-text', { page: 'dns' });
    const text = formatDNSShareText(
      {
        verdictState,
        verdictTitle: status.title,
        verdict: current.verdict,
        note,
        exit: current.exit,
        exitNetworkName: current.exit?.ip
          ? (dnsProvider(current.exit.network) || m.common.unknownNetwork)
          : m.common.unavailable,
        exitRegion: current.exit?.ip ? dnsRegion(current.exit.country, locale) : '',
        exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit), locale) : '',
        providers: providers.map((p) => ({
          name: p.name,
          regions: joinList(p.regions.map((r) => dnsRegion(r.country, locale)), locale),
          type: p.type,
          suspect: p.suspect,
        })),
        comparisonSummary: comparison?.summary,
      },
      locale,
    );
    toast?.show(await copyShareText(text, locale));
  }
</script>

{#snippet exitReadout(exit: DNSExit | null, pending: boolean, waiting = false)}
  <div class="reading">
    <p class="field" title={m.common.exitIPTooltip}><i class="fa-solid fa-globe" aria-hidden="true"></i>{m.common.exitIP}</p>
    {#if exit?.ip}
      {@const networkType = exitTypeLabel(dnsExitType(exit), locale)}
      <div class="network-identity exit-identity">
        <span class="network-name">{dnsProvider(exit.network) || m.common.unknownNetwork}</span>
        <span class="region">{dnsRegion(exit.country, locale)}</span>
      </div>
      <div class="mt-0.5 mb-1.5">
        <ExitIPAddress
          {locale}
          ip={exit.ip}
          revealed={revealExit}
          onToggleReveal={() => { revealExit = !revealExit; }}
          onCopyFeedback={(message) => toast?.show(message)}
        />
      </div>
      {#if networkType || exit.asn}
        <div class="exit-meta">
          {#if networkType}<span class="badge">{networkType}</span>{/if}
          {#if exit.asn}<span class="font-mono">{exit.asn}</span>{/if}
        </div>
      {/if}
    {:else}
      <p class="readout-placeholder">{pending ? m.common.fetching : waiting ? m.common.idle : m.common.unavailable}</p>
    {/if}
  </div>
{/snippet}

<AppHeader
  page="dns"
  {locale}
  onCopyText={handleCopyText}
  onScreenshot={handleScreenshot}
  onRescan={() => run('retest')}
  isScanning={!ready || busy}
  ready={!!current}
  failureText={error?.title}
/>

<main class="report-main flex-1 min-w-0">
  <header class="mb-6">
    <h1 class="report-title"><span class="text-radar">{m.dns.titleAccent}</span>{m.dns.titleRest}</h1>
  </header>

  <div role="status" class="sr-only" data-nosnippet>
    {busy ? (current ? m.dns.page.srRetesting : m.dns.page.srTesting) : error?.title ?? (current ? status.title : '')}
  </div>

  <div class="diagnostic-layout">
    {#if current}
      <section class="glass-card result-card" aria-label={m.dns.page.resultAria} data-nosnippet>
        {#if error}
          <div class="result-notice">
            <i class="fa-solid fa-circle-info" aria-hidden="true"></i>
            <span>{failure === 'empty' ? m.dns.failures.noticeEmpty : m.dns.failures.noticeOther(error.note)}</span>
          </div>
        {/if}

        <DNSFlowStage
          {locale}
          exit={current.exit}
          {verdictState}
          providers={flowProviders}
          resolverCount={current.resolvers.length}
          {revealExit}
          {busy}
          {liveProgress}
          hasResult={true}
        />
        {#if busy}
          <div class="progress-track" role="progressbar" aria-label={m.dns.page.retestProgressAria}><span></span></div>
        {/if}

        <header class="diagnosis" style:--verdict-color={status.color} style:--verdict-soft={status.soft}>
          <p class="card-label">{busy ? m.common.retestingPrevious : error ? m.common.previousResult : m.common.thisScan}</p>
          <p class="verdict-title"><span class="dot" class:loading-dot={busy} aria-hidden="true"></span>{status.title}</p>
          {#if note}<p class="diagnosis-note">{note}</p>{/if}
        </header>

        <div class="columns" class:retesting={busy}>
          {@render exitReadout(current.exit, false)}
          <div class="reading">
            <p class="field"><i class="fa-solid fa-diagram-project" aria-hidden="true"></i>DNS</p>
            <ul class="network-list" aria-label={m.dns.page.networkListAria}>
              {#each providers as provider (provider.key)}
                <li class="network-row" class:suspect-network={provider.suspect}>
                  <div class="network-identity">
                    {#if provider.suspect}
                      <span class="suspect-dot" aria-hidden="true"></span><span class="sr-only">{statusMap.suspected.title}</span>
                    {/if}
                    <span class="network-name">{provider.name}</span>
                    <span class="region">{joinList(provider.regions.map((region) => dnsRegion(region.country, locale)), locale)}</span>
                  </div>
                  <div class="network-tags">
                    {#if provider.type === 'public'}
                      <span class="badge">{m.dns.page.tagPublic}</span>
                    {:else if provider.type === 'access'}
                      <span class="badge">{m.dns.page.tagAccess}</span>
                    {/if}
                  </div>
                </li>
              {/each}
            </ul>
          </div>
        </div>
        {#if providers.length}
          <details class="resolver-details">
            <summary>
              <span class="summary-name">
                <i class="chevron fa-solid fa-chevron-right" aria-hidden="true"></i>
                {m.dns.page.detailsTitle}
              </span>
              <span class="address-count"><span class="font-mono">{m.dns.page.addressCount(current.resolvers.length)}</span></span>
            </summary>
            <div class="resolver-groups">
              <ul aria-label={m.dns.page.groupedAria}>
                {#each providers as provider (provider.key)}
                  <li class="resolver-provider">
                    <div class="network-identity provider-heading">
                      <span class="network-name">{provider.name}</span>
                      {#if provider.regions.length === 1}<span class="region">{dnsRegion(provider.regions[0].country, locale)}</span>{/if}
                    </div>
                    <div class="region-groups">
                      {#each provider.regions as region (region.country)}
                        <div>
                          {#if provider.regions.length > 1}<p class="region mb-1.5">{dnsRegion(region.country, locale)}</p>{/if}
                          <ul class="resolver-addresses">
                            {#each region.samples as resolver (resolver.ip)}
                              <li class="resolver-address">
                                {#if current.verdict?.suspect_ips?.includes(resolver.ip)}
                                  <span class="suspect-dot" aria-hidden="true"></span><span class="sr-only">{statusMap.suspected.title}</span>
                                {/if}
                                <span class="ip-address">{resolver.ip}</span>
                                {#if !error && comparison?.addedIPs.has(resolver.ip)}<span class="badge new">{m.common.added}</span>{/if}
                              </li>
                            {/each}
                          </ul>
                        </div>
                      {/each}
                    </div>
                  </li>
                {/each}
              </ul>
              {#if !error && comparison?.removedIPs.length}
                <div class="old-addresses">
                  <p class="region">{busy ? m.common.noLongerSeenPrevious : m.common.noLongerSeenCurrent}</p>
                  <ul class="resolver-addresses mt-2">
                    {#each comparison.removedIPs as ip (ip)}
                      <li class="resolver-address"><span class="ip-address">{ip}</span></li>
                    {/each}
                  </ul>
                </div>
              {/if}
            </div>
          </details>
        {/if}
      </section>
    {:else if error}
      <section class="glass-card result-card" aria-label={error.title} data-nosnippet>
        <div class="failure-card">
          <p class="verdict-title">{error.title}</p>
          <p class="diagnosis-note">{error.note}</p>
          <button
            type="button"
            onclick={() => run('retest')}
            class="mt-3 text-sm px-3.5 py-1.5 rounded-lg transition-colors cursor-pointer"
            style="color:var(--radar-400); background:var(--sig-soft)"
          >{m.common.rescan}</button>
        </div>
      </section>
    {:else}
      {@const isWaiting = ready && !busy}
      <section class="glass-card result-card" aria-label={isWaiting ? m.dns.page.waitingTitle : m.dns.page.testingTitle} data-nosnippet>
        <DNSFlowStage
          {locale}
          exit={pendingExit}
          verdictState="unknown"
          providers={[]}
          resolverCount={0}
          {revealExit}
          busy={!isWaiting}
          {liveProgress}
          hasResult={false}
        />
        {#if !isWaiting}
          <div class="progress-track" role="progressbar" aria-label={m.dns.page.progressAria}><span></span></div>
        {/if}
        <header class="diagnosis">
          {#if isWaiting}
            <p class="verdict-title"><span class="dot" aria-hidden="true"></span>{m.dns.page.waitingTitle}</p>
            <p class="diagnosis-note">{m.dns.page.waitingDesc}</p>
            <button
              type="button"
              onclick={() => run('manual')}
              class="mt-3 text-sm px-3.5 py-1.5 rounded-lg transition-colors cursor-pointer"
              style="color:var(--radar-400); background:var(--sig-soft)"
            >{m.common.startScan}</button>
          {:else}
            <p class="verdict-title"><span class="dot loading-dot" aria-hidden="true"></span>{m.dns.page.testingTitle}</p>
            <p class="diagnosis-note">{m.dns.page.testingDesc}</p>
          {/if}
        </header>
        <div class="columns">
          {@render exitReadout(pendingExit, !isWaiting, isWaiting)}
          <div class="reading">
            <p class="field"><i class="fa-solid fa-diagram-project" aria-hidden="true"></i>DNS</p>
            {#if isWaiting}
              <p class="readout-placeholder">{m.common.idle}</p>
            {:else}
              <div class="mt-3 h-5 w-36 rounded bg-white/8" aria-hidden="true"></div>
              <div class="mt-2 h-3 w-21 rounded bg-white/5" aria-hidden="true"></div>
            {/if}
          </div>
        </div>
      </section>
    {/if}

    <div class="side-stack">
      {#if comparison && !error}
        <aside class="glass-card side-card" aria-labelledby="dns-comparison-title" data-nosnippet>
          <p class="side-heading" id="dns-comparison-title">
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
                <i class="fa-solid fa-arrow-right" aria-hidden="true"></i><span class="sr-only">{m.common.changedToSr}</span>
                <strong>{status.title}</strong>
              </p>
            {/if}
            <dl class="change-list">
              <div><dt>DNS</dt><dd>
                {#each comparison.dnsChanges as { prefix = '', removed = [], text = '' }}
                  <p>
                    {prefix}{#each removed as { name, region }, index}
                      {index > 0 ? (locale === 'en' ? ', ' : '、') : ''}{#if region}
                        {name}{locale === 'en' && region.startsWith(' ') ? ' ' : ''}<s>{locale === 'en' ? region.trim() : region}</s>
                      {:else}
                        <s>{name}</s>
                      {/if}
                    {/each}{text}
                  </p>
                {:else}
                  {m.dns.page.providerUnchanged}
                {/each}
              </dd></div>
              <div><dt>{m.common.exitIP}</dt><dd>{comparison.exitChange || m.common.unchanged}</dd></div>
            </dl>
          {/if}
        </aside>
      {/if}
      <aside class="glass-card side-card" aria-labelledby="dns-info-title">
        <p class="side-heading" id="dns-info-title">
          <i class="fa-solid fa-circle-info" aria-hidden="true"></i>{m.common.howItWorks}
        </p>
        <ul class="rule-note">
          <li>{m.dns.page.rules[0]}</li>
          <li>{m.dns.page.rules[1]}<strong>{statusMap.suspected.title}</strong></li>
          <li>
            {m.dns.page.rules[2]}<strong>{statusMap.clear.title}</strong>{locale === 'en' ? ' — see the ' : '，完整口径见 '}<a href={localePath('/about/#dns', locale)} class="text-(--radar-400) underline underline-offset-4">{m.common.fullCriteriaLink}</a>
          </li>
        </ul>
      </aside>
    </div>
  </div>
</main>

<ReportToast bind:this={toast} />

<style>
  summary:focus-visible {
    outline: 2px solid var(--radar-400); outline-offset: -2px;
  }
  .result-card { border-radius: 0.875rem; overflow: hidden; }
  .diagnosis {
    padding: 1.125rem 1.25rem 1.25rem; border-bottom: 1px solid var(--line);
    background: linear-gradient(110deg, var(--verdict-soft, var(--sig-soft)), transparent 66%);
  }
  .card-label, .side-heading { color: var(--slate-400); font-size: 0.8125rem; font-weight: 500; }
  .card-label { margin-bottom: 0.5rem; }
  .diagnostic-layout { display: grid; gap: 1rem; align-items: start; }
  .diagnostic-layout > * { min-width: 0; }
  .side-stack { display: grid; gap: 1rem; }
  .side-card { border-radius: 0.875rem; padding: 1.125rem 1.25rem; }
  .side-heading { display: flex; align-items: center; gap: 0.5rem; }
  .side-heading i { font-size: 0.75rem; }
  .comparison-summary { margin-top: 0.825rem; font-weight: 600; color: var(--slate-200); }
  .rule-note { margin-top: 0.7rem; color: var(--slate-300); font-size: 0.875rem; line-height: 1.65; }
  .rule-note { list-style: disc; padding-inline-start: 1rem; }
  .rule-note li + li { margin-top: 0.375rem; }
  .rule-note strong { color: var(--slate-200); font-weight: 600; }
  .change-list { margin-top: 0.875rem; display: grid; gap: 0.5rem; font-size: 0.875rem; }
  .verdict-change + .change-list { border-top: 1px solid var(--line); padding-top: 0.875rem; }
  .change-list > div { display: grid; grid-template-columns: minmax(4rem, auto) minmax(0, 1fr); align-items: baseline; gap: 0.5rem; }
  .change-list dt { color: var(--slate-400); font-size: 0.8125rem; white-space: nowrap; }
  .change-list dd { color: var(--slate-200); overflow-wrap: anywhere; }
  .change-list s { color: var(--slate-400); }
  .verdict-title {
    position: relative; min-width: 0; color: white; font-size: 1.125rem;
    font-weight: 700; line-height: 1.4;
  }
  .diagnosis .verdict-title { padding-inline-start: 0.975rem; }
  .verdict-title .dot {
    position: absolute; inset-inline-start: 0; top: 0.47em;
    background: var(--verdict-color, var(--sig)); --_soft: var(--verdict-soft, var(--sig-soft));
  }
  .verdict-change { margin-top: 0.75rem; display: flex; align-items: center; flex-wrap: wrap; gap: 0.55rem; color: var(--slate-400); font-size: 0.875rem; }
  .verdict-change i { font-size: 0.7rem; }
  .verdict-change strong { color: var(--change-color); font-weight: 600; font-size: 1.125rem; }
  .diagnosis-note { margin-top: 0.45rem; color: var(--slate-300); font-size: 0.875rem; line-height: 1.55; text-wrap: pretty; }
  .result-notice {
    display: flex; align-items: baseline; gap: 0.5rem; padding: 0.55rem 1.25rem;
    background: var(--warn-soft); border-bottom: 1px solid var(--line); color: var(--slate-200); font-size: 0.8125rem;
  }
  .result-notice i { color: var(--warn); flex: none; }
  .failure-card { padding: 1.25rem; }
  .columns { display: grid; transition: opacity 0.2s ease; }
  .columns.retesting { opacity: 0.55; }
  .columns > :global(.reading) + :global(.reading) { border-top: 1px solid var(--line); }
  .reading { min-width: 0; padding: 1rem 1.25rem; }
  .field { display: flex; align-items: center; gap: 0.5rem; color: var(--slate-400); font-size: 0.8125rem; font-weight: 500; }
  .field i { font-size: 0.75rem; }
  .network-identity { display: flex; min-width: 0; flex-wrap: wrap; align-items: baseline; gap: 0.12rem 0.55rem; }
  .network-name { min-width: 0; color: var(--slate-200); font-size: 1rem; font-weight: 600; overflow-wrap: anywhere; }
  .region { color: var(--slate-400); font-size: 0.8125rem; }
  .exit-identity, .readout-placeholder { margin-top: 0.625rem; }
  .readout-placeholder { color: var(--slate-400); font-size: 0.9375rem; }
  .exit-meta, .network-tags { display: flex; flex-wrap: wrap; align-items: center; gap: 0.375rem 0.55rem; }
  .network-tags { flex: none; }
  .exit-meta { color: var(--slate-400); font-size: 0.75rem; }
  .network-list { margin-top: 0.2rem; }
  .network-row { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 0.25rem 0.75rem; padding: 0.5rem 0; }
  .network-row + .network-row { border-top: 1px solid var(--line); }
  .network-row:last-child { padding-bottom: 0; }
  .suspect-network .network-name { color: var(--leak); }
  .suspect-dot { flex: none; display: inline-block; width: 0.33rem; height: 0.33rem; border-radius: 50%; background: var(--leak); }
  .network-identity .suspect-dot { align-self: center; }
  .badge {
    display: inline-block; padding: 0.02rem 0.4rem; border: 1px solid var(--line); border-radius: 0.3rem;
    color: var(--slate-300); font-size: 0.6875rem; line-height: 1.5; white-space: nowrap;
  }
  .badge.new { color: var(--radar-400); background: var(--sig-soft); border-color: transparent; }
  .resolver-details { border-top: 1px solid var(--line); }
  summary { display: flex; justify-content: space-between; align-items: center; gap: 0.75rem; padding: 0.8rem 1.25rem; list-style: none; cursor: pointer; }
  summary:hover { background: rgba(255, 255, 255, 0.025); }
  summary::-webkit-details-marker { display: none; }
  .summary-name { display: flex; align-items: center; gap: 0.55rem; color: var(--slate-200); font-size: 0.875rem; font-weight: 500; }
  .chevron { color: var(--slate-400); font-size: 0.625rem; }
  details[open] .chevron { transform: rotate(90deg); }
  .address-count { color: var(--slate-400); font-size: 0.75rem; white-space: nowrap; }
  .resolver-groups { padding: 0 1.25rem 0.875rem; }
  .resolver-provider { display: grid; gap: 0.45rem 1rem; border-top: 1px solid var(--line); padding-block: 0.625rem; }
  .provider-heading { align-self: start; }
  .provider-heading .network-name { font-size: 0.875rem; }
  .region-groups { display: grid; min-width: 0; gap: 0.5rem; }
  .resolver-addresses { display: flex; flex-direction: column; flex-wrap: wrap; gap: 0.25rem 0.5rem; }
  .resolver-address {
    display: flex; align-items: center; flex-wrap: wrap; gap: 0.3rem 0.45rem; min-width: 0; max-width: 100%;
    padding: 0.16rem 0;
  }
  .ip-address {
    display: block; min-width: 0; max-width: 100%; overflow-x: auto; white-space: nowrap;
    font-family: 'JetBrains Mono', ui-monospace, monospace; font-size: 0.75rem; color: var(--slate-200);
  }
  .old-addresses { padding-top: 0.625rem; border-top: 1px solid var(--line); }
  .old-addresses .ip-address { color: var(--slate-400); }
  .progress-track { height: 0.1875rem; background: var(--line); overflow: hidden; }
  .progress-track > span { display: block; width: 30%; height: 100%; border-radius: 999px; background: var(--radar-400); opacity: 0.6; }
  @media (min-width: 40rem) {
    .diagnosis, .reading, .failure-card { padding-inline: 1.5rem; }
    .side-card { padding: 1.25rem 1.5rem; }
    .verdict-title { font-size: 1.375rem; }
    .columns { grid-template-columns: minmax(0, 5fr) minmax(0, 7fr); }
    .columns > :global(.reading) + :global(.reading) { border-top: 0; border-left: 1px solid var(--line); }
    summary, .resolver-groups { padding-inline: 1.5rem; }
    .resolver-provider { grid-template-columns: minmax(0, 12rem) minmax(0, 1fr); align-items: baseline; }
    .resolver-addresses { flex-direction: row; }
    .resolver-address { padding: 0.22rem 0.5rem; border: 1px solid var(--line); border-radius: 0.3rem; background: rgba(2, 6, 23, 0.22); }
  }
  @media (min-width: 64rem) {
    .diagnostic-layout { grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); gap: 0 1.5rem; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .progress-track > span { animation: sampling 1.7s ease-in-out infinite alternate; }
    @keyframes sampling { from { transform: translateX(-60%); } to { transform: translateX(220%); } }
  }
</style>
