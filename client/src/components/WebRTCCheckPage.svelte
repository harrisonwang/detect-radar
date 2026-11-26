<script lang="ts">
  import { onMount } from 'svelte';
  import { getMessages, localePath, type Locale } from '../i18n';
  import AppHeader from './report/AppHeader.svelte';
  import ExitIPAddress from './report/ExitIPAddress.svelte';
  import ReportToast from './report/ReportToast.svelte';
  import WebRTCFlowStage from './report/WebRTCFlowStage.svelte';
  import {
    dnsExitType,
    dnsProviderName,
    dnsRegion,
    exitTypeLabel,
  } from '../lib/dnsPresentation';
  import { canonicalIP, isIPv6Address, sameIP, sameIPv6Prefix64 } from '../lib/ip';
  import {
    compareWebRTC,
    getWebrtcStatus,
    localScopeLabel,
    measureWebRTC,
    webrtcHistory,
    webrtcVerdictNote,
    type WebRTCCheck,
    type WebRTCHistory,
    type WebRTCProgress,
  } from '../lib/webrtcCheck';
  import { emptyHistory } from '../lib/recentHistory';
  import { trackEvent } from '../lib/analytics';
  import { isAutomatedCrawler } from '../lib/crawler';
  import { renderWebRTCShareCard, saveShareImage } from '../lib/toolShareCard';
  import { copyShareText, formatWebRTCShareText } from '../lib/shareSummary';
  import type { DNSExit, WebRTCNetworkInfo } from '../lib/types';

  interface Props {
    locale?: Locale;
  }
  let { locale = 'zh-CN' }: Props = $props();

  let m = $derived(getMessages(locale));
  let statusMap = $derived(getWebrtcStatus(locale));

  let history = $state.raw<WebRTCHistory>(emptyHistory<WebRTCCheck>());
  let ready = $state(false);
  let busy = $state(false);
  let pendingExit = $state.raw<DNSExit | null>(null);
  let liveProgress = $state.raw<WebRTCProgress | null>(null);
  let revealExit = $state(true);
  let capturing = $state(false);
  let toast: ReturnType<typeof ReportToast> | undefined = $state();

  let current = $derived(history.current);
  let comparison = $derived(
    current && history.previous ? compareWebRTC(history.previous, current, locale) : null,
  );
  let verdict = $derived(current?.verdict);
  let verdictNote = $derived(current ? webrtcVerdictNote(current, locale) : '');
  let status = $derived(verdict ? statusMap[verdict.state] : statusMap.unknown);
  let suspectSet = $derived(new Set((verdict?.suspectIPs ?? []).map((ip) => canonicalIP(ip))));
  let publicInfoByIP = $derived.by(() => {
    const map = new Map<string, WebRTCNetworkInfo>();
    for (const info of current?.publicCandidates ?? []) {
      const key = canonicalIP(info.ip);
      if (key && !map.has(key)) map.set(key, info);
    }
    return map;
  });

  function formatCandidateMeta(info: WebRTCNetworkInfo | undefined): string {
    if (!info) return '';
    const parts: string[] = [];
    if (info.country) {
      const region = dnsRegion(info.country, locale);
      if (region && region !== m.common.unknownRegion) parts.push(region);
    }
    const provider = dnsProviderName(info.network ?? '', locale);
    if (provider) parts.push(provider);
    if (info.asn) parts.push(info.asn);
    return parts.join(' · ');
  }

  let localScopeByIP = $derived.by(() => {
    const map = new Map<string, (typeof current extends infer C ? C extends WebRTCCheck ? C['candidates'][number]['localScope'] : never : never)>();
    if (!current) return map;
    for (const c of current.candidates) {
      if (c.kind === 'local' && c.localScope && !map.has(c.address)) {
        map.set(c.address, c.localScope);
      }
    }
    return map;
  });
  let removedAll = $derived(
    comparison ? [...comparison.removedPublicIPs, ...comparison.removedLocalIPs] : [],
  );

  onMount(() => {
    if (isAutomatedCrawler()) {
      ready = true;
      return;
    }
    async function restore() {
      try {
        history = await webrtcHistory();
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
    if (mode === 'retest') trackEvent('scan-retest', { page: 'webrtc' });
    else trackEvent('scan-start', { page: 'webrtc', trigger: mode });
    busy = true;
    pendingExit = null;
    liveProgress = null;
    try {
      const result = await measureWebRTC(
        (exit) => {
          pendingExit = exit;
        },
        (progress) => {
          liveProgress = progress;
        },
        locale,
      );
      try {
        history = await webrtcHistory(result, history.current);
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
      const blob = await renderWebRTCShareCard({
        time: current.time,
        verdictState: verdict.state,
        verdictTitle: status.title,
        verdictReason: verdict.reason,
        verdictNote,
        exit: current.exit,
        exitNetworkName: current.exit?.ip
          ? (dnsProviderName(current.exit.network, locale) || m.common.unknownNetwork)
          : m.common.unavailable,
        exitRegion: current.exit?.ip ? dnsRegion(current.exit.country, locale) : '',
        exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit), locale) : '',
        apiSupported: current.apiSupported,
        publicIPs: current.publicIPs,
        suspectIPs: verdict.suspectIPs,
        localIPs: current.localIPs.map((ip) => ({
          ip,
          scope: localScopeLabel(localScopeByIP.get(ip), locale),
        })),
        mdnsCount: current.mdnsHosts.length,
        ipv6Global: current.ipv6Global,
        locale,
      });
      const shareMsg = await saveShareImage(blob, 'detectradar-webrtc', locale);
      trackEvent('share-save-image', { page: 'webrtc' });
      toast?.show(shareMsg);
    } catch (err) {
      toast?.show(m.common.imageFailed((err as Error).message));
    } finally {
      capturing = false;
    }
  }

  async function handleCopyText() {
    if (!current || busy || !verdict) return;
    trackEvent('share-copy-text', { page: 'webrtc' });
    const text = formatWebRTCShareText(
      {
        verdictState: verdict.state,
        verdictTitle: status.title,
        verdictReason: verdict.reason,
        verdictNote,
        exit: current.exit,
        exitNetworkName: current.exit?.ip
          ? (dnsProviderName(current.exit.network, locale) || m.common.unknownNetwork)
          : m.common.unavailable,
        exitRegion: current.exit?.ip ? dnsRegion(current.exit.country, locale) : '',
        exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit), locale) : '',
        apiSupported: current.apiSupported,
        publicIPs: current.publicIPs,
        suspectIPs: verdict.suspectIPs,
        localIPs: current.localIPs.map((ip) => ({
          ip,
          scope: localScopeLabel(localScopeByIP.get(ip), locale),
        })),
        mdnsCount: current.mdnsHosts.length,
        ipv6Global: current.ipv6Global,
        comparisonSummary: comparison?.summary,
      },
      locale,
    );
    toast?.show(await copyShareText(text, locale));
  }
</script>

{#snippet exitReadout(exit: DNSExit | null, pending: boolean, waiting = false)}
  <div class="reading">
    <p class="field" title={m.common.exitIPTooltip}>
      <i class="fa-solid fa-globe" aria-hidden="true"></i>{m.common.exitIP}
    </p>
    {#if exit?.ip}
      {@const networkType = exitTypeLabel(dnsExitType(exit), locale)}
      <div class="network-identity exit-identity">
        <span class="network-name">{dnsProviderName(exit.network, locale) || m.common.unknownNetwork}</span>
        <span class="region">{dnsRegion(exit.country, locale)}</span>
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
  page="webrtc"
  {locale}
  onCopyText={handleCopyText}
  onScreenshot={handleScreenshot}
  onRescan={() => run('retest')}
  isScanning={!ready || busy}
  ready={!!current}
/>

<main class="report-main flex-1 min-w-0">
  <header class="mb-6">
    <h1 class="report-title"><span class="text-radar">{m.webrtc.titleAccent}</span>{m.webrtc.titleRest}</h1>
  </header>

  <div role="status" class="sr-only" data-nosnippet>
    {busy
      ? current
        ? m.webrtc.page.srRetesting
        : m.webrtc.page.srTesting
      : current
        ? status.title
        : ''}
  </div>

  <div class="diagnostic-layout">
    {#if current}
      <section class="glass-card result-card" aria-label={m.webrtc.page.resultAria} data-nosnippet>
        {#if current.exitError}
          <div class="result-notice">
            <i class="fa-solid fa-circle-info" aria-hidden="true"></i>
            <span>
              {current.exitError === 'limited'
                ? m.webrtc.page.noticeLimited
                : m.webrtc.page.noticeUnavailable}
            </span>
          </div>
        {/if}

        <WebRTCFlowStage
          {locale}
          exit={current.exit}
          verdictState={verdict?.state ?? 'unknown'}
          verdictReason={verdict?.reason}
          {verdictNote}
          apiSupported={current.apiSupported}
          publicIPs={current.publicIPs}
          suspectIPs={verdict?.suspectIPs ?? []}
          localIPs={current.localIPs}
          mdnsHosts={current.mdnsHosts}
          {revealExit}
          {busy}
          {liveProgress}
          hasResult={true}
        />

        {#if busy}
          <div class="progress-track" role="progressbar" aria-label={m.webrtc.page.retestProgressAria}>
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
          {@render exitReadout(current.exit, false)}

          <div class="reading">
            <p class="field">
              <i class="fa-solid fa-tower-broadcast" aria-hidden="true"></i>WebRTC
            </p>

            <ul class="network-list" aria-label={m.webrtc.page.listAria}>
              <li class="network-row network-stack">
                <div class="row-head">
                  <span class="row-label">{m.webrtc.page.publicIPLabel}</span>
                  {#if !current.apiSupported}
                    <span class="badge">{m.webrtc.page.disabledBadge}</span>
                  {:else if current.publicIPs.length === 0}
                    <span class="region">{m.webrtc.page.noPublicIP}</span>
                  {/if}
                </div>
                {#if current.publicIPs.length > 0}
                  <ul class="candidate-list" aria-label={m.webrtc.page.publicIPAria}>
                    {#each current.publicIPs as ip (ip)}
                      {@const canonIP = canonicalIP(ip)}
                      {@const candInfo = publicInfoByIP.get(canonIP)}
                      {@const candMeta = formatCandidateMeta(candInfo)}
                      {@const isSuspect = suspectSet.has(canonIP)}
                      {@const matchesExit =
                        candInfo?.relation === 'matches-exit' || sameIP(ip, current.exit?.ip)}
                      {@const samePrefix =
                        candInfo?.relation === 'same-prefix' ||
                        (!matchesExit && sameIPv6Prefix64(ip, current.exit?.ip))}
                      {@const isDualStack =
                        candInfo?.relation === 'dual-stack' ||
                        (!isSuspect && !matchesExit && !samePrefix && verdict?.reason === 'dual-stack')}
                      {@const isSameNetwork =
                        candInfo?.relation === 'same-network' ||
                        (!isSuspect && !matchesExit && !samePrefix && verdict?.reason === 'same-network')}
                      {@const isV6 = isIPv6Address(ip)}
                      <li class="candidate-item" class:suspect-network={isSuspect}>
                        <div class="candidate-main">
                          <div class="network-identity">
                            {#if isSuspect}
                              <span class="suspect-dot" aria-hidden="true"></span>
                              <span class="sr-only">{statusMap.leaked.title}</span>
                            {/if}
                            <span class="ip-address">{canonIP}</span>
                          </div>
                          {#if candMeta}
                            <span class="candidate-meta">{candMeta}</span>
                          {/if}
                        </div>
                        <div class="network-tags">
                          {#if isV6}<span class="badge">IPv6</span>{/if}
                          {#if isSuspect}
                            <span class="badge danger">{m.webrtc.page.tagDifferent}</span>
                          {:else if matchesExit}
                            <span class="badge">{m.webrtc.page.tagMatch}</span>
                          {:else if samePrefix}
                            <span class="badge">{m.webrtc.page.tagSamePrefix}</span>
                          {:else if isDualStack}
                            <span class="badge">{m.webrtc.page.tagSameISP}</span>
                          {:else if isSameNetwork}
                            <span class="badge">{m.webrtc.page.tagSharedExit}</span>
                          {/if}
                          {#if comparison?.addedPublicIPs.has(canonIP)}
                            <span class="badge new">{m.common.added}</span>
                          {/if}
                        </div>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>

              <li class="network-row network-stack">
                <div class="row-head">
                  <span class="row-label">{m.webrtc.page.localAddrLabel}</span>
                  {#if !current.apiSupported}
                    <span class="region">{m.webrtc.page.apiDisabled}</span>
                  {:else if current.localIPs.length === 0 && current.mdnsHosts.length > 0}
                    <span class="badge">{m.webrtc.page.mdnsHidden}</span>
                  {:else if current.localIPs.length === 0}
                    <span class="region">{m.webrtc.page.localNotExposed}</span>
                  {/if}
                </div>
                {#if current.localIPs.length > 0}
                  <ul class="candidate-list" aria-label={m.webrtc.page.localAddrAria}>
                    {#each current.localIPs as ip (ip)}
                      <li class="candidate-item">
                        <span class="ip-address">{ip}</span>
                        <div class="network-tags">
                          <span class="badge">{localScopeLabel(localScopeByIP.get(ip), locale)}</span>
                          {#if comparison?.addedLocalIPs.has(ip)}
                            <span class="badge new">{m.common.added}</span>
                          {/if}
                        </div>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>

              <li class="network-row">
                <div class="row-head w-full">
                  <span class="row-label">{m.webrtc.page.ipv6AddrLabel}</span>
                  {#if current.ipv6Global}
                    <span class="ip-address">{current.ipv6Global}</span>
                  {:else if !current.apiSupported}
                    <span class="region">{m.webrtc.page.ipv6Skipped}</span>
                  {:else}
                    <span class="region">{m.webrtc.page.ipv6None}</span>
                  {/if}
                </div>
              </li>
            </ul>
          </div>
        </div>

        {#if current.candidates.length > 0 || removedAll.length > 0}
          <details class="resolver-details">
            <summary>
              <span class="summary-name">
                <i class="chevron fa-solid fa-chevron-right" aria-hidden="true"></i>
                {m.webrtc.page.detailsTitle}
              </span>
              <span class="address-count">
                <span class="font-mono">{m.webrtc.page.addressCount(current.candidates.length)}</span>
              </span>
            </summary>

            <div class="resolver-groups">
              {#if current.candidates.length > 0}
                <ul class="detail-list" aria-label={m.webrtc.page.rawListAria}>
                  {#each current.candidates as cand (`${cand.address}|${cand.type}|${cand.protocol}`)}
                    <li class="detail-row">
                      <span class="ip-address">{cand.address}</span>
                      <div class="network-tags">
                        <span class="badge font-mono">{cand.protocol.toUpperCase()}</span>
                        <span class="badge font-mono">{cand.type}</span>
                        {#if cand.kind === 'mdns'}
                          <span class="badge">mDNS</span>
                        {:else if cand.kind === 'local'}
                          <span class="badge">{localScopeLabel(cand.localScope, locale)}</span>
                        {:else if suspectSet.has(canonicalIP(cand.address))}
                          <span class="badge danger">{m.webrtc.page.tagDifferent}</span>
                        {:else}
                          <span class="badge">{m.webrtc.page.tagPublic}</span>
                        {/if}
                      </div>
                    </li>
                  {/each}
                </ul>
              {/if}

              {#if removedAll.length > 0}
                <div class="old-addresses">
                  <p class="region">{busy ? m.common.noLongerSeenPrevious : m.common.noLongerSeenCurrent}</p>
                  <ul class="resolver-addresses mt-2">
                    {#each removedAll as ip (ip)}
                      <li class="resolver-address"><span class="ip-address">{ip}</span></li>
                    {/each}
                  </ul>
                </div>
              {/if}
            </div>
          </details>
        {/if}
      </section>
    {:else}
      {@const isWaiting = ready && !busy}
      <section class="glass-card result-card" aria-label={isWaiting ? m.webrtc.page.waitingTitle : m.webrtc.page.testingTitle} data-nosnippet>
        <WebRTCFlowStage
          {locale}
          exit={pendingExit}
          verdictState="unknown"
          apiSupported={true}
          publicIPs={[]}
          suspectIPs={[]}
          localIPs={[]}
          mdnsHosts={[]}
          {revealExit}
          busy={!isWaiting}
          {liveProgress}
          hasResult={false}
        />
        {#if !isWaiting}
          <div class="progress-track" role="progressbar" aria-label={m.webrtc.page.progressAria}>
            <span></span>
          </div>
        {/if}
        <header class="diagnosis">
          {#if isWaiting}
            <p class="verdict-title">
              <span class="dot" aria-hidden="true"></span>{m.webrtc.page.waitingTitle}
            </p>
            <p class="diagnosis-note">{m.webrtc.page.waitingDesc}</p>
            <button
              type="button"
              onclick={() => run('manual')}
              class="mt-3 text-sm px-3.5 py-1.5 rounded-lg transition-colors cursor-pointer"
              style="color:var(--radar-400); background:var(--sig-soft)"
            >{m.common.startScan}</button>
          {:else}
            <p class="verdict-title">
              <span class="dot loading-dot" aria-hidden="true"></span>{m.webrtc.page.testingTitle}
            </p>
            <p class="diagnosis-note">{m.webrtc.page.testingDesc}</p>
          {/if}
        </header>
        <div class="columns">
          {@render exitReadout(pendingExit, !isWaiting, isWaiting)}
          <div class="reading">
            <p class="field">
              <i class="fa-solid fa-tower-broadcast" aria-hidden="true"></i>WebRTC
            </p>
            {#if isWaiting}
              <p class="readout-placeholder">{m.common.idle}</p>
            {:else}
              <div class="mt-3 h-5 w-40 rounded bg-white/8" aria-hidden="true"></div>
              <div class="mt-2 h-3 w-24 rounded bg-white/5" aria-hidden="true"></div>
            {/if}
          </div>
        </div>
      </section>
    {/if}

    <div class="side-stack">
      {#if comparison}
        <aside class="glass-card side-card" aria-labelledby="webrtc-comparison-title" data-nosnippet>
          <p class="side-heading" id="webrtc-comparison-title">
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
                <dt>{m.webrtc.page.publicIPLabel}</dt>
                <dd>{comparison.publicChange}</dd>
              </div>
              <div>
                <dt>{m.webrtc.page.localAddrLabel}</dt>
                <dd>{comparison.localChange}</dd>
              </div>
              <div>
                <dt>{m.common.exitIP}</dt>
                <dd>{comparison.exitChange}</dd>
              </div>
            </dl>
          {/if}
        </aside>
      {/if}

      <aside class="glass-card side-card" aria-labelledby="webrtc-info-title">
        <p class="side-heading" id="webrtc-info-title">
          <i class="fa-solid fa-circle-info" aria-hidden="true"></i>{m.common.howItWorks}
        </p>
        <ul class="rule-note">
          <li>{m.webrtc.page.rules[0]}</li>
          <li>
            {m.webrtc.page.rules[1]}<strong>{statusMap.leaked.title}</strong>{locale === 'en' ? ' — see ' : '，完整口径见 '}<a href={localePath('/about/#webrtc', locale)} class="text-(--radar-400) underline underline-offset-4">{m.common.fullCriteriaLink}</a>
          </li>
          <li>
            {m.webrtc.page.rules[2]}<a
              href={localePath('/ipv6-leak-test/', locale)}
              onclick={() => trackEvent('tool-open', { from: 'webrtc', to: 'ipv6', source: 'note' })}
              class="text-(--radar-400) underline underline-offset-4"
            >{m.webrtc.page.ipv6ToolLink}</a>
          </li>
        </ul>
      </aside>
    </div>
  </div>
</main>

<ReportToast bind:this={toast} />

<style>
  summary:focus-visible {
    outline: 2px solid var(--radar-400);
    outline-offset: -2px;
  }
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
  .exit-meta,
  .network-tags {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.375rem 0.45rem;
  }
  .network-tags {
    flex: none;
  }
  .exit-meta {
    color: var(--slate-400);
    font-size: 0.75rem;
  }
  .network-list {
    margin-top: 0.35rem;
  }
  .network-row {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: center;
    gap: 0.35rem 0.75rem;
    padding: 0.625rem 0;
  }
  .network-row.network-stack {
    flex-direction: column;
    align-items: stretch;
  }
  .network-row + .network-row {
    border-top: 1px solid var(--line);
  }
  .network-row:last-child {
    padding-bottom: 0;
  }
  .row-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.25rem 0.75rem;
    min-width: 0;
  }
  .row-label {
    color: var(--slate-200);
    font-size: 0.875rem;
    font-weight: 600;
  }
  .candidate-list {
    display: grid;
    gap: 0.375rem;
    min-width: 0;
  }
  .candidate-item {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.25rem 0.5rem;
    min-width: 0;
  }
  .candidate-main {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    min-width: 0;
  }
  .candidate-meta {
    color: var(--slate-400);
    font-size: 0.75rem;
    line-height: 1.4;
    overflow-wrap: anywhere;
  }
  .suspect-network .ip-address {
    color: var(--leak);
  }
  .suspect-dot {
    flex: none;
    display: inline-block;
    width: 0.33rem;
    height: 0.33rem;
    border-radius: 50%;
    background: var(--leak);
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
  .badge.new {
    color: var(--radar-400);
    background: var(--sig-soft);
    border-color: transparent;
  }
  .badge.danger {
    color: var(--leak);
    background: var(--leak-soft);
    border-color: transparent;
  }
  .resolver-details {
    border-top: 1px solid var(--line);
  }
  summary {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
    padding: 0.8rem 1.25rem;
    list-style: none;
    cursor: pointer;
  }
  summary:hover {
    background: rgba(255, 255, 255, 0.025);
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .summary-name {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    color: var(--slate-200);
    font-size: 0.875rem;
    font-weight: 500;
  }
  .chevron {
    color: var(--slate-400);
    font-size: 0.625rem;
  }
  details[open] .chevron {
    transform: rotate(90deg);
  }
  .address-count {
    color: var(--slate-400);
    font-size: 0.75rem;
    white-space: nowrap;
  }
  .resolver-groups {
    padding: 0 1.25rem 0.875rem;
  }
  .detail-list {
    display: grid;
    border-top: 1px solid var(--line);
  }
  .detail-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.3rem 0.75rem;
    padding: 0.5rem 0;
    min-width: 0;
  }
  .detail-row + .detail-row {
    border-top: 1px solid var(--line);
  }
  .resolver-addresses {
    display: flex;
    flex-direction: column;
    flex-wrap: wrap;
    gap: 0.25rem 0.5rem;
  }
  .resolver-address {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.3rem 0.45rem;
    min-width: 0;
    max-width: 100%;
    padding: 0.16rem 0;
  }
  .ip-address {
    display: block;
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    white-space: nowrap;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 0.8125rem;
    color: var(--slate-200);
  }
  .old-addresses {
    padding-top: 0.625rem;
    border-top: 1px solid var(--line);
  }
  .old-addresses .ip-address {
    color: var(--slate-400);
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
      grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
    }
    .columns > :global(.reading) + :global(.reading) {
      border-top: 0;
      border-left: 1px solid var(--line);
    }
    summary,
    .resolver-groups {
      padding-inline: 1.5rem;
    }
    .resolver-addresses {
      flex-direction: row;
    }
    .resolver-address {
      padding: 0.22rem 0.5rem;
      border: 1px solid var(--line);
      border-radius: 0.3rem;
      background: rgba(2, 6, 23, 0.22);
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
