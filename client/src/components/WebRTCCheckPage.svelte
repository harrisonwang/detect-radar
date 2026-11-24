<script lang="ts">
  import { onMount } from 'svelte';
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
  import {
    compareWebRTC,
    localScopeLabel,
    measureWebRTC,
    webrtcHistory,
    webrtcStatus,
    type WebRTCCheck,
    type WebRTCHistory,
    type WebRTCProgress,
  } from '../lib/webrtcCheck';
  import { emptyHistory } from '../lib/recentHistory';
  import { isAutomatedCrawler } from '../lib/crawler';
  import { renderWebRTCShareCard, saveShareImage } from '../lib/toolShareCard';
  import { copyShareText, formatWebRTCShareText } from '../lib/shareSummary';
  import type { DNSExit } from '../lib/types';

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
    current && history.previous ? compareWebRTC(history.previous, current) : null,
  );
  let verdict = $derived(current?.verdict);
  let status = $derived(verdict ? webrtcStatus[verdict.state] : webrtcStatus.unknown);
  let suspectSet = $derived(new Set(verdict?.suspectIPs ?? []));
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
        /* 浏览器限制存储时，在当前页生命周期内继续支持前后对比。 */
      }
      ready = true;
      const navigation = performance.getEntriesByType('navigation')[0] as
        | PerformanceNavigationTiming
        | undefined;
      if (!history.current || navigation?.type === 'navigate') await run();
    }
    void restore();
  });

  async function run() {
    if (busy || !ready) return;
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
        verdictNote: verdict.note,
        exit: current.exit,
        exitNetworkName: current.exit?.ip ? (dnsProviderName(current.exit.network) || '网络未知') : '未获取',
        exitRegion: current.exit?.ip ? dnsRegion(current.exit.country) : '',
        exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit)) : '',
        apiSupported: current.apiSupported,
        publicIPs: current.publicIPs,
        suspectIPs: verdict.suspectIPs,
        localIPs: current.localIPs.map((ip) => ({
          ip,
          scope: localScopeLabel(localScopeByIP.get(ip)),
        })),
        mdnsCount: current.mdnsHosts.length,
        ipv6Global: current.ipv6Global,
      });
      toast?.show(await saveShareImage(blob, 'detectradar-webrtc'));
    } catch (err) {
      toast?.show('分享图生成失败：' + (err as Error).message);
    } finally {
      capturing = false;
    }
  }
  async function handleCopyText() {
    if (!current || busy || !verdict) return;
    const text = formatWebRTCShareText({
      verdictState: verdict.state,
      verdictTitle: status.title,
      verdictReason: verdict.reason,
      verdictNote: verdict.note,
      exit: current.exit,
      exitNetworkName: current.exit?.ip ? (dnsProviderName(current.exit.network) || '网络未知') : '未获取',
      exitRegion: current.exit?.ip ? dnsRegion(current.exit.country) : '',
      exitTypeLabel: current.exit?.ip ? exitTypeLabel(dnsExitType(current.exit)) : '',
      apiSupported: current.apiSupported,
      publicIPs: current.publicIPs,
      suspectIPs: verdict.suspectIPs,
      localIPs: current.localIPs.map((ip) => ({
        ip,
        scope: localScopeLabel(localScopeByIP.get(ip)),
      })),
      mdnsCount: current.mdnsHosts.length,
      ipv6Global: current.ipv6Global,
      comparisonSummary: comparison?.summary,
    });
    toast?.show(await copyShareText(text));
  }
</script>

{#snippet exitReadout(exit: DNSExit | null, pending: boolean, waiting = false)}
  <div class="reading">
    <p class="field" title="你访问本站时使用的公网 IP">
      <i class="fa-solid fa-globe" aria-hidden="true"></i>出口 IP
    </p>
    {#if exit?.ip}
      {@const networkType = exitTypeLabel(dnsExitType(exit))}
      <div class="network-identity exit-identity">
        <span class="network-name">{dnsProviderName(exit.network) || '网络未知'}</span>
        <span class="region">{dnsRegion(exit.country)}</span>
      </div>
      <div class="mt-0.5 mb-1.5">
        <ExitIPAddress
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
      <p class="readout-placeholder">{pending ? '正在获取…' : waiting ? '待检测' : '未获取'}</p>
    {/if}
  </div>
{/snippet}

<AppHeader
  page="webrtc"
  onCopyText={handleCopyText}
  onScreenshot={handleScreenshot}
  onRescan={run}
  isScanning={!ready || busy}
  ready={!!current}
/>

<main class="report-main flex-1 min-w-0">
  <header class="mb-6">
    <h1 class="report-title"><span class="text-radar">WebRTC</span> 泄露检测</h1>
  </header>

  <div role="status" class="sr-only">
    {busy
      ? current
        ? '正在重新检测 WebRTC。'
        : '正在检测 WebRTC。'
      : current
        ? status.title
        : ''}
  </div>

  <div class="diagnostic-layout">
    {#if current}
      <section class="glass-card result-card" aria-label="WebRTC 检测结果">
        {#if current.exitError}
          <div class="result-notice">
            <i class="fa-solid fa-circle-info" aria-hidden="true"></i>
            <span>
              {current.exitError === 'limited'
                ? '出口 IP 请求过于频繁，暂未获取到用于比对的出口地址。'
                : '出口 IP 服务暂时不可用，仅展示本地 WebRTC 收集结果。'}
            </span>
          </div>
        {/if}

        <WebRTCFlowStage
          exit={current.exit}
          verdictState={verdict?.state ?? 'unknown'}
          verdictReason={verdict?.reason}
          verdictNote={verdict?.note}
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
          <div class="progress-track" role="progressbar" aria-label="WebRTC 重新检测进度">
            <span></span>
          </div>
        {/if}

        <header
          class="diagnosis"
          style:--verdict-color={status.color}
          style:--verdict-soft={status.soft}
        >
          <p class="card-label">{busy ? '正在重新检测 · 上次结果' : '本次检测'}</p>
          <p class="verdict-title">
            <span class="dot" class:loading-dot={busy} aria-hidden="true"></span>{status.title}
          </p>
          {#if verdict?.note}
            <p class="diagnosis-note">{verdict.note}</p>
          {/if}
        </header>

        <div class="columns" class:retesting={busy}>
          {@render exitReadout(current.exit, false)}

          <div class="reading">
            <p class="field">
              <i class="fa-solid fa-tower-broadcast" aria-hidden="true"></i>WebRTC
            </p>

            <ul class="network-list" aria-label="检测到的 WebRTC 地址">
              <li class="network-row network-stack">
                <div class="row-head">
                  <span class="row-label">公网 IP</span>
                  {#if !current.apiSupported}
                    <span class="badge">WebRTC 已禁用</span>
                  {:else if current.publicIPs.length === 0}
                    <span class="region">未测到公网 IP</span>
                  {/if}
                </div>
                {#if current.publicIPs.length > 0}
                  <ul class="candidate-list" aria-label="WebRTC 公网 IP">
                    {#each current.publicIPs as ip (ip)}
                      {@const isSuspect = suspectSet.has(ip)}
                      {@const matchesExit =
                        !!current.exit?.ip && ip.toLowerCase() === current.exit.ip.toLowerCase()}
                      {@const isV6 = ip.includes(':')}
                      <li class="candidate-item" class:suspect-network={isSuspect}>
                        <div class="network-identity">
                          {#if isSuspect}
                            <span class="suspect-dot" aria-hidden="true"></span>
                            <span class="sr-only">泄露</span>
                          {/if}
                          <span class="ip-address">{ip}</span>
                        </div>
                        <div class="network-tags">
                          {#if isV6}<span class="badge">IPv6</span>{/if}
                          {#if isSuspect}
                            <span class="badge danger">与出口不同</span>
                          {:else if matchesExit}
                            <span class="badge">与出口一致</span>
                          {:else if verdict?.reason === 'dual-stack'}
                            <span class="badge">双栈 IP</span>
                          {/if}
                          {#if comparison?.addedPublicIPs.has(ip)}
                            <span class="badge new">新增</span>
                          {/if}
                        </div>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>

              <li class="network-row network-stack">
                <div class="row-head">
                  <span class="row-label">内网地址</span>
                  {#if !current.apiSupported}
                    <span class="region">接口已禁用</span>
                  {:else if current.localIPs.length === 0 && current.mdnsHosts.length > 0}
                    <span class="badge">mDNS 已隐藏（*.local）</span>
                  {:else if current.localIPs.length === 0}
                    <span class="region">未暴露</span>
                  {/if}
                </div>
                {#if current.localIPs.length > 0}
                  <ul class="candidate-list" aria-label="WebRTC 内网地址">
                    {#each current.localIPs as ip (ip)}
                      <li class="candidate-item">
                        <span class="ip-address">{ip}</span>
                        <div class="network-tags">
                          <span class="badge">{localScopeLabel(localScopeByIP.get(ip))}</span>
                          {#if comparison?.addedLocalIPs.has(ip)}
                            <span class="badge new">新增</span>
                          {/if}
                        </div>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>

              <li class="network-row">
                <div class="row-head w-full">
                  <span class="row-label">IPv6 地址</span>
                  {#if current.ipv6Global}
                    <span class="ip-address">{current.ipv6Global}</span>
                  {:else if !current.apiSupported}
                    <span class="region">禁用 WebRTC 时未采集</span>
                  {:else}
                    <span class="region">未测到全局 IPv6</span>
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
                WebRTC 地址
              </span>
              <span class="address-count">
                <span class="font-mono">{current.candidates.length}</span> 个地址
              </span>
            </summary>

            <div class="resolver-groups">
              {#if current.candidates.length > 0}
                <ul class="detail-list" aria-label="WebRTC 地址原始记录">
                  {#each current.candidates as cand (`${cand.address}|${cand.type}|${cand.protocol}`)}
                    <li class="detail-row">
                      <span class="ip-address">{cand.address}</span>
                      <div class="network-tags">
                        <span class="badge font-mono">{cand.protocol.toUpperCase()}</span>
                        <span class="badge font-mono">{cand.type}</span>
                        {#if cand.kind === 'mdns'}
                          <span class="badge">mDNS</span>
                        {:else if cand.kind === 'local'}
                          <span class="badge">{localScopeLabel(cand.localScope)}</span>
                        {:else if suspectSet.has(cand.address)}
                          <span class="badge danger">与出口不同</span>
                        {:else}
                          <span class="badge">公网</span>
                        {/if}
                      </div>
                    </li>
                  {/each}
                </ul>
              {/if}

              {#if removedAll.length > 0}
                <div class="old-addresses">
                  <p class="region">{busy ? '上次未再测到的地址' : '本次未再测到的地址'}</p>
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
      <section class="glass-card result-card" aria-label={isWaiting ? '待检测 WebRTC' : '正在检测 WebRTC'}>
        <WebRTCFlowStage
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
          <div class="progress-track" role="progressbar" aria-label="WebRTC 检测进度">
            <span></span>
          </div>
        {/if}
        <header class="diagnosis">
          {#if isWaiting}
            <p class="verdict-title">
              <span class="dot" aria-hidden="true"></span>待检测 WebRTC
            </p>
            <p class="diagnosis-note">点击开始后将并发检测网页出口与 WebRTC 地址。</p>
            <button
              type="button"
              onclick={run}
              class="mt-3 text-sm px-3.5 py-1.5 rounded-lg transition-colors cursor-pointer"
              style="color:var(--radar-400); background:var(--sig-soft)"
            >开始检测</button>
          {:else}
            <p class="verdict-title">
              <span class="dot loading-dot" aria-hidden="true"></span>正在检测 WebRTC
            </p>
            <p class="diagnosis-note">正在并发检测网页出口与 WebRTC 地址。</p>
          {/if}
        </header>
        <div class="columns">
          {@render exitReadout(pendingExit, !isWaiting, isWaiting)}
          <div class="reading">
            <p class="field">
              <i class="fa-solid fa-tower-broadcast" aria-hidden="true"></i>WebRTC
            </p>
            {#if isWaiting}
              <p class="readout-placeholder">待检测</p>
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
        <aside class="glass-card side-card" aria-labelledby="webrtc-comparison-title">
          <p class="side-heading" id="webrtc-comparison-title">
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
                  <s>{webrtcStatus[comparison.previousVerdict].title}</s>
                {:else}
                  <span>{webrtcStatus[comparison.previousVerdict].title}</span>
                {/if}
                <i class="fa-solid fa-arrow-right" aria-hidden="true"></i>
                <span class="sr-only">变为</span>
                <strong>{status.title}</strong>
              </p>
            {/if}
            <dl class="change-list">
              <div>
                <dt>公网 IP</dt>
                <dd>{comparison.publicChange}</dd>
              </div>
              <div>
                <dt>内网地址</dt>
                <dd>{comparison.localChange}</dd>
              </div>
              <div>
                <dt>出口 IP</dt>
                <dd>{comparison.exitChange}</dd>
              </div>
            </dl>
          {/if}
        </aside>
      {/if}

      <aside class="glass-card side-card" aria-labelledby="webrtc-info-title">
        <p class="side-heading" id="webrtc-info-title">
          <i class="fa-solid fa-circle-info" aria-hidden="true"></i>检测说明
        </p>
        <ul class="rule-note">
          <li>结果来自当前浏览器的 WebRTC 检测，仅在本机完成比对。</li>
          <li>测到与当前出口不同的公网 IP（非正常双栈），判为<strong>泄露</strong>。</li>
          <li>如需独立核对真实 IPv4 / IPv6 双栈出口，请使用 <a href="/ipv6-leak-test/" class="text-(--radar-400) underline underline-offset-4">IPv6 泄露检测</a>（不依赖 WebRTC）。</li>
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
    grid-template-columns: 4.25rem minmax(0, 1fr);
    align-items: baseline;
    gap: 0.5rem;
  }
  .change-list dt {
    color: var(--slate-400);
    font-size: 0.8125rem;
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
