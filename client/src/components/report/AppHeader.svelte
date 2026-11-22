<script lang="ts">
  interface Props {
    page?: "home" | "dns" | "webrtc" | "static";
    onScreenshot?: () => void;
    onShare?: () => void;
    onRescan?: () => void;
    isScanning?: boolean;
    ready?: boolean;
    failureText?: string;
  }
  let { page = "home", onScreenshot, onRescan, isScanning = false, ready = false, failureText }: Props = $props();
  let statusText = $derived(isScanning ? '检测中' : failureText || (ready ? '检测完成' : '待检测'));
  let statusColor = $derived(isScanning ? 'var(--warn)' : ready && !failureText ? 'var(--sig)' : 'var(--unknown)');
</script>

<header class="border-b border-slate-800/60 bg-space-950/80 sticky top-0 z-50 backdrop-blur-md">
  <div class="header-wrap max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="header-row">
      <a href="/" class="brand flex items-center gap-1.5 sm:gap-3 group">
        <span class="relative size-6 sm:size-8 flex items-center justify-center">
          <i class="fa-solid fa-crosshairs text-(--radar-400) text-xl group-hover:rotate-180 transition-transform duration-700"></i>
          <span class="absolute inset-0 bg-(--radar-400)/2 rounded-full animate-ping-slow"></span>
        </span>
        <span class="text-lg sm:text-xl font-bold tracking-tight text-white font-mono">Detect<span class="text-(--radar-400)">Radar</span></span>
      </a>

      <nav aria-label="主导航" class="site-nav">
        <a href="/" aria-current={page === 'home' ? 'page' : undefined}>网络环境检测</a>
        <a href="/dns-leak-test/" aria-current={page === 'dns' ? 'page' : undefined}>DNS 泄露检测</a>
        <a href="/webrtc-leak-test/" aria-current={page === 'webrtc' ? 'page' : undefined}>WebRTC 泄露检测</a>
      </nav>

      {#if page !== "static"}
        <div class="report-actions">
          <span class="flex items-center gap-1.5 text-xs font-mono text-(--slate-500) whitespace-nowrap" role="status">
            <span aria-hidden="true" class="dot" style:background={statusColor} style:--_soft="transparent"></span>
            {statusText}
          </span>
          {#if ready || page === "dns" || page === "webrtc"}
            <div class="icon-actions">
              <button type="button" onclick={onRescan} disabled={isScanning} class="hbtn" title="重新检测" aria-label="重新检测">
                <i class="fa-solid fa-rotate-right text-sm" aria-hidden="true"></i>
              </button>
              {#if onScreenshot}
                <button type="button" onclick={onScreenshot} class="hbtn"
                  title="生成分享图（IP 自动脱敏）" aria-label="生成分享图">
                  <i class="fa-solid fa-camera text-sm" aria-hidden="true"></i>
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</header>

<style>
  .header-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    grid-template-areas: "brand nav actions";
    column-gap: 0.5rem;
    align-items: center;
    min-height: 4rem;
  }
  .brand { grid-area: brand; justify-self: start; }
  .report-actions {
    grid-area: actions;
    justify-self: end;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    flex-wrap: wrap;
    max-width: 100%;
    gap: 0 0.5rem;
  }
  .icon-actions { display: flex; column-gap: inherit; }
  .site-nav {
    grid-area: nav;
    place-self: center;
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    align-items: center;
    gap: 0.25rem clamp(0.75rem, 1.5vw, 1.35rem);
  }
  .site-nav a {
    display: flex;
    align-items: center;
    min-height: 44px;
    padding-inline: 0.25rem;
    border-radius: 6px;
    color: var(--slate-300);
    font-size: 0.875rem;
    font-weight: 500;
    line-height: 1.35;
    white-space: nowrap;
  }
  @media (max-width: 26rem) {
    .site-nav {
      gap: 0.15rem 0.65rem;
    }
    .site-nav a {
      font-size: 0.8125rem;
      min-height: 40px;
    }
  }
  .site-nav a[aria-current="page"] { color: var(--radar-400); font-weight: 600; }
  @media (hover: hover) {
    .site-nav a:hover { color: white; }
  }
  .site-nav a:focus-visible, .hbtn:focus-visible {
    outline: 2px solid var(--radar-400);
    outline-offset: 2px;
  }
  @media (max-width: 40rem) {
    .header-row {
      grid-template-areas: "brand actions" "nav nav";
      grid-template-columns: auto minmax(0, 1fr);
      grid-template-rows: minmax(2.5rem, auto) auto;
      padding-top: 0.375rem;
    }
  }

  .hbtn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 38px;
    height: 38px;
    border-radius: 8px;
    color: var(--slate-400);
    background: rgba(30, 41, 59, 0.5);
    transition: all 0.18s ease;
    cursor: pointer;
  }
  .hbtn:hover:not(:disabled) {
    color: var(--radar-400);
    background: var(--slate-700);
  }
  .hbtn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  @media (max-width: 23rem) {
    .report-actions { column-gap: 0.25rem; }
    .hbtn { width: 32px; height: 36px; }
    .header-wrap { padding-inline: 0.625rem; }
    .brand { gap: 0.25rem; }
    .brand > span:last-child { font-size: 1rem; }
  }
  @media (max-width: 22rem) {
    .brand > span:first-child { display: none; }
  }
</style>
