<script lang="ts">
  import { onMount } from 'svelte';
  import AppHeader from './report/AppHeader.svelte';
  import DNSFlowStage from './report/DNSFlowStage.svelte';
  import ExitIPAddress from './report/ExitIPAddress.svelte';
  import ReportToast from './report/ReportToast.svelte';
  import { compareDNS, DNSCheckError, dnsNetworks, dnsProviderGroups, measureDNS, type DNSProgress, withDNSAssessment } from '../lib/dnsCheck';
  import { dnsHistory, emptyDNSHistory, type DNSHistory } from '../lib/dnsHistory';
  import { dnsExitType, dnsProviderName, dnsRegion, dnsStatus, dnsVerdictNote, exitTypeLabel } from '../lib/dnsPresentation';
  import { renderDNSShareCard, saveShareImage } from '../lib/toolShareCard';
  import type { DNSExit } from '../lib/types';

  const failures = {
    empty: { title: dnsStatus.unobserved.title, note: '网络较慢或测试请求被拦截时，可能测不到结果。' },
    limited: { title: '检测未完成', note: '请求过于频繁，请稍后再试。' },
    unavailable: { title: '检测未完成', note: '检测服务暂时连不上。' },
  };
  let history = $state.raw<DNSHistory>(emptyDNSHistory);
  let ready = $state(false);
  let busy = $state(false);
  let failure = $state<keyof typeof failures | null>(null);
  let pendingExit = $state.raw<DNSExit | null>(null);
  let liveProgress = $state.raw<DNSProgress | null>(null);
  let revealExit = $state(true);
  let capturing = $state(false);
  let toast: ReturnType<typeof ReportToast> | undefined = $state();

  let current = $derived(history.current);
  let networks = $derived(current ? dnsNetworks(current) : null);
  let providers = $derived(networks ? dnsProviderGroups(networks.values()) : []);
  let comparison = $derived(current && history.previous && networks
    ? compareDNS(history.previous, current, networks) : null);
  let verdictState = $derived(current?.verdict?.state ?? 'unknown');
  let status = $derived(dnsStatus[verdictState]);
  let publicDNSAbroad = $derived(verdictState === 'clear' && current?.exit?.country
    && providers.length > 0 && providers.every((p) => p.type === 'public')
    && providers.some((p) => p.regions.some((r) => r.country && r.country !== current?.exit?.country)));
  let note = $derived(publicDNSAbroad ? '公共 DNS 和出口 IP 的归属地可以不同。' : dnsVerdictNote(current?.verdict));
  let error = $derived(failure ? failures[failure] : null);
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
    async function restore() {
      try {
        const saved = await dnsHistory();
        const [current, previous] = await Promise.all([
          saved.current && withDNSAssessment(saved.current),
          saved.previous && withDNSAssessment(saved.previous),
        ]);
        history = { current, previous };
      } catch { /* In browsers with storage restrictions, compare in this page. */ }
      ready = true;
      const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
      if (!history.current || navigation?.type === 'navigate') await run();
    }
    void restore();
  });

  async function run() {
    if (busy || !ready) return;
    busy = true;
    failure = null;
    pendingExit = null;
    liveProgress = null;
    try {
      const result = await measureDNS(
        (exit) => { pendingExit = exit; },
        (progress) => { liveProgress = progress; },
      );
      try {
        const saved = await dnsHistory(result, history.current);
        const previous = saved.previous && await withDNSAssessment(saved.previous);
        history = { current: result, previous };
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
        exitNetworkName: current.exit?.ip ? (dnsProviderName(current.exit.network) || '网络未知') : '未获取',
        exitRegion: current.exit?.ip ? dnsRegion(current.exit.country) : '',
        exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit)) : '',
        providers: providers.map((p) => ({
          name: p.name,
          regions: p.regions.map((r) => dnsRegion(r.country)).join('、'),
          type: p.type,
          suspect: p.suspect,
        })),
        resolverCount: current.resolvers.length,
      });
      toast?.show(await saveShareImage(blob, 'detectradar-dns'));
    } catch (err) {
      toast?.show('分享图生成失败：' + (err as Error).message);
    } finally {
      capturing = false;
    }
  }
</script>

{#snippet exitReadout(exit: DNSExit | null, pending: boolean)}
  <div class="reading">
    <p class="field" title="你访问本站时使用的公网 IP"><i class="fa-solid fa-globe" aria-hidden="true"></i>出口 IP</p>
    {#if exit?.ip}
      {@const networkType = exitTypeLabel(dnsExitType(exit))}
      <div class="network-identity exit-identity">
        <span class="network-name">{dnsProviderName(exit.network) || '网络未知'}</span>
        <span class="region">{dnsRegion(exit.country)}</span>
      </div>
      <div class="mt-0.5 mb-1.5">
        <ExitIPAddress ip={exit.ip} revealed={revealExit}
          onToggleReveal={() => { revealExit = !revealExit; }}
          onCopyFeedback={(message) => toast?.show(message)} />
      </div>
      {#if networkType || exit.asn}
        <div class="exit-meta">
          {#if networkType}<span class="badge">{networkType}</span>{/if}
          {#if exit.asn}<span class="font-mono">{exit.asn}</span>{/if}
        </div>
      {/if}
    {:else}
      <p class="readout-placeholder">{pending ? '正在获取…' : '未获取'}</p>
    {/if}
  </div>
{/snippet}

<AppHeader page="dns" onScreenshot={handleScreenshot} onRescan={run} isScanning={!ready || busy} ready={!!current} failureText={error?.title} />

<main class="report-main flex-1 min-w-0">
  <header class="mb-6">
    <h1 class="report-title"><span class="text-radar">DNS</span> 泄露检测</h1>
  </header>

  <div role="status" class="sr-only">
    {busy ? (current ? '正在重新检测 DNS。' : '正在检测 DNS。') : error?.title ?? (current ? status.title : '')}
  </div>

  <div class="diagnostic-layout">
    {#if current}
      <section class="glass-card result-card" aria-label="DNS 检测结果">
        {#if error}
          <div class="result-notice">
            <i class="fa-solid fa-circle-info" aria-hidden="true"></i>
            <span>{failure === 'empty' ? '这次未测到 DNS，' : error.note}已保留上次结果。</span>
          </div>
        {/if}

        <DNSFlowStage
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
          <div class="progress-track" role="progressbar" aria-label="DNS 重新检测进度"><span></span></div>
        {/if}

        <header class="diagnosis" style:--verdict-color={status.color} style:--verdict-soft={status.soft}>
          <p class="card-label">{busy ? '正在重新检测 · 上次结果' : error ? '上次结果' : '本次检测'}</p>
          <p class="verdict-title"><span class="dot" class:loading-dot={busy} aria-hidden="true"></span>{status.title}</p>
          {#if note}<p class="diagnosis-note">{busy || error ? note.replace(/本次 ?/g, '') : note}</p>{/if}
        </header>

        <div class="columns" class:retesting={busy}>
          {@render exitReadout(current.exit, false)}
          <div class="reading">
            <p class="field"><i class="fa-solid fa-diagram-project" aria-hidden="true"></i>DNS</p>
            <ul class="network-list" aria-label="检测到的 DNS 网络">
              {#each providers as provider (provider.key)}
                <li class="network-row" class:suspect-network={provider.suspect}>
                  <div class="network-identity">
                    {#if provider.suspect}
                      <span class="suspect-dot" aria-hidden="true"></span><span class="sr-only">泄露</span>
                    {/if}
                    <span class="network-name">{provider.name}</span>
                    <span class="region">{provider.regions.map((region) => dnsRegion(region.country)).join('、')}</span>
                  </div>
                  <div class="network-tags">
                    {#if provider.type === 'public'}
                      <span class="badge">公共 DNS</span>
                    {:else if provider.type === 'access'}
                      <span class="badge">运营商 DNS</span>
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
                DNS 地址
              </span>
              <span class="address-count"><span class="font-mono">{current.resolvers.length}</span> 个地址</span>
            </summary>
            <div class="resolver-groups">
              <ul aria-label="按服务商分组的 DNS 地址">
                {#each providers as provider (provider.key)}
                  <li class="resolver-provider">
                    <div class="network-identity provider-heading">
                      <span class="network-name">{provider.name}</span>
                      {#if provider.regions.length === 1}<span class="region">{dnsRegion(provider.regions[0].country)}</span>{/if}
                    </div>
                    <div class="region-groups">
                      {#each provider.regions as region (region.country)}
                        <div>
                          {#if provider.regions.length > 1}<p class="region mb-1.5">{dnsRegion(region.country)}</p>{/if}
                          <ul class="resolver-addresses">
                            {#each region.samples as resolver (resolver.ip)}
                              <li class="resolver-address">
                                {#if current.verdict?.suspect_ips?.includes(resolver.ip)}
                                  <span class="suspect-dot" aria-hidden="true"></span><span class="sr-only">泄露</span>
                                {/if}
                                <span class="ip-address">{resolver.ip}</span>
                                {#if !error && comparison?.addedIPs.has(resolver.ip)}<span class="badge new">新增</span>{/if}
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
                  <p class="region">{busy ? '上次未再测到的地址' : '本次未再测到的地址'}</p>
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
      <section class="glass-card result-card" aria-label={error.title}>
        <div class="failure-card">
          <p class="verdict-title">{error.title}</p>
          <p class="diagnosis-note">{error.note}</p>
        </div>
      </section>
    {:else}
      <section class="glass-card result-card" aria-label="正在检测">
        <DNSFlowStage
          exit={pendingExit}
          verdictState="unknown"
          providers={[]}
          resolverCount={0}
          {revealExit}
          busy={true}
          {liveProgress}
          hasResult={false}
        />
        <div class="progress-track" role="progressbar" aria-label="DNS 检测进度"><span></span></div>
        <header class="diagnosis">
          <p class="verdict-title"><span class="dot loading-dot" aria-hidden="true"></span>正在检测 DNS</p>
          <p class="diagnosis-note">正在并发采集网页出口与权威 DNS 回溯。</p>
        </header>
        <div class="columns">
          {@render exitReadout(pendingExit, true)}
          <div class="reading">
            <p class="field"><i class="fa-solid fa-diagram-project" aria-hidden="true"></i>DNS</p>
            <div class="mt-3 h-5 w-36 rounded bg-white/8" aria-hidden="true"></div>
            <div class="mt-2 h-3 w-21 rounded bg-white/5" aria-hidden="true"></div>
          </div>
        </div>
      </section>
    {/if}

    <div class="side-stack">
      {#if comparison && !error}
        <aside class="glass-card side-card" aria-labelledby="dns-comparison-title">
          <p class="side-heading" id="dns-comparison-title">
            <i class="fa-solid fa-arrow-right-arrow-left" aria-hidden="true"></i>
            {busy ? '上次对比' : '和上次比'}
          </p>
          {#if comparison.summary}
            <p class="comparison-summary">{comparison.summary}</p>
          {:else}
            {#if comparison.previousVerdict}
              <p class="verdict-change" style:--change-color={status.color}>
                <span class="sr-only">前次检测</span>
                {#if comparison.improved}
                  <s>{dnsStatus[comparison.previousVerdict].title}</s>
                {:else}
                  <span>{dnsStatus[comparison.previousVerdict].title}</span>
                {/if}
                <i class="fa-solid fa-arrow-right" aria-hidden="true"></i><span class="sr-only">变为</span>
                <strong>{status.title}</strong>
              </p>
            {/if}
            <dl class="change-list">
              <div><dt>DNS</dt><dd>
                {#each comparison.dnsChanges as { prefix = '', removed = [], text = '' }}
                  <p>
                    {busy ? prefix.replace(/^本次/, '') : prefix}{#each removed as { name, region }, index}
                      {index > 0 ? '、' : ''}{#if region}
                        {name}<s>{region}</s>
                      {:else}
                        <s>{name}</s>
                      {/if}
                    {/each}{busy ? text.replace(/^本次/, '') : text}
                  </p>
                {:else}
                  服务商未变
                {/each}
              </dd></div>
              <div><dt>出口 IP</dt><dd>{(busy ? comparison.exitChange.replace(/^本次/, '当时') : comparison.exitChange) || '未变'}</dd></div>
            </dl>
          {/if}
        </aside>
      {/if}
      <aside class="glass-card side-card" aria-labelledby="dns-info-title">
        <p class="side-heading" id="dns-info-title">
          <i class="fa-solid fa-circle-info" aria-hidden="true"></i>检测说明
        </p>
        <ul class="rule-note">
          <li>结果来自当前浏览器的 DNS 检测。</li>
          <li>出口 IP 在海外，测到<span class="nowrap">国内运营商 DNS</span>，判为<strong>泄露</strong>。</li>
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
  .nowrap { white-space: nowrap; }
  .rule-note strong { color: var(--slate-200); font-weight: 600; }
  .change-list { margin-top: 0.875rem; display: grid; gap: 0.5rem; font-size: 0.875rem; }
  .verdict-change + .change-list { border-top: 1px solid var(--line); padding-top: 0.875rem; }
  .change-list > div { display: grid; grid-template-columns: 3.25rem minmax(0, 1fr); align-items: baseline; gap: 0.5rem; }
  .change-list dt { color: var(--slate-400); font-size: 0.8125rem; }
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
