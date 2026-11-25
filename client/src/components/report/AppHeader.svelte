<script lang="ts">
  import { trackEvent, type SpecialistTool } from '../../lib/analytics';

  interface Props {
    page?: "home" | "dns" | "webrtc" | "ipv6" | "static";
    onCopyText?: () => void;
    onScreenshot?: () => void;
    onRescan?: () => void;
    isScanning?: boolean;
    ready?: boolean;
    failureText?: string;
  }
  let {
    page = "home",
    onCopyText,
    onScreenshot,
    onRescan,
    isScanning = false,
    ready = false,
    failureText,
  }: Props = $props();

  let statusText = $derived(isScanning ? '检测中' : failureText || (ready ? '检测完成' : '待检测'));
  let statusColor = $derived(isScanning ? 'var(--warn)' : ready && !failureText ? 'var(--sig)' : 'var(--unknown)');
  let canShare = $derived(Boolean(onCopyText || onScreenshot));
  let shareDisabled = $derived(isScanning || !ready);

  let shareOpen = $state(false);
  let menuOpen = $state(false);
  let shareWrap: HTMLDivElement | undefined = $state();
  let headerEl: HTMLElement | undefined = $state();

  $effect(() => {
    if (shareDisabled && shareOpen) {
      shareOpen = false;
    }
  });

  function toggleShareMenu() {
    if (shareDisabled) return;
    if (onCopyText && onScreenshot) {
      const next = !shareOpen;
      shareOpen = next;
      if (next) menuOpen = false;
      return;
    }
    menuOpen = false;
    if (onCopyText) onCopyText();
    else if (onScreenshot) onScreenshot();
  }

  function toggleNavMenu() {
    const next = !menuOpen;
    menuOpen = next;
    if (next) shareOpen = false;
  }

  function closeNavMenu() {
    menuOpen = false;
  }

  function handleToolNavClick(target: SpecialistTool) {
    closeNavMenu();
    if (page !== 'static' && page !== target) {
      trackEvent('tool-open', { from: page, to: target, source: 'nav' });
    }
  }

  function handleWindowPointerDown(e: PointerEvent) {
    const target = e.target as Node | null;
    if (shareOpen && shareWrap && target && !shareWrap.contains(target)) {
      shareOpen = false;
    }
    if (menuOpen && headerEl && target && !headerEl.contains(target)) {
      menuOpen = false;
    }
  }

  function handleWindowKeyDown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      shareOpen = false;
      menuOpen = false;
    }
  }

  function runCopyText() {
    shareOpen = false;
    onCopyText?.();
  }

  function runScreenshot() {
    shareOpen = false;
    onScreenshot?.();
  }
</script>

<svelte:window onpointerdown={handleWindowPointerDown} onkeydown={handleWindowKeyDown} />

<header bind:this={headerEl} class="border-b border-slate-800/60 bg-space-950/80 sticky top-0 z-50 backdrop-blur-md">
  <div class="header-wrap max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="header-row">
      <a href="/" class="brand flex items-center gap-1.5 sm:gap-3 group">
        <span class="relative size-6 sm:size-8 flex items-center justify-center">
          <i class="fa-solid fa-crosshairs text-(--radar-400) text-xl group-hover:rotate-180 transition-transform duration-700"></i>
          <span class="absolute inset-0 bg-(--radar-400)/2 rounded-full animate-ping-slow"></span>
        </span>
        <span class="text-lg sm:text-xl font-bold tracking-tight text-white font-mono">Detect<span class="text-(--radar-400)">Radar</span></span>
      </a>

      <nav id="site-nav" aria-label="主导航" class="site-nav" class:is-open={menuOpen}>
        <a href="/" aria-current={page === 'home' ? 'page' : undefined} onclick={closeNavMenu}>网络环境检测</a>
        <a
          href="/dns-leak-test/"
          aria-current={page === 'dns' ? 'page' : undefined}
          onclick={() => handleToolNavClick('dns')}
          data-umami-event={page === 'static' ? 'tool-open' : undefined}
          data-umami-event-from={page === 'static' ? 'static' : undefined}
          data-umami-event-to={page === 'static' ? 'dns' : undefined}
          data-umami-event-source={page === 'static' ? 'nav' : undefined}
        >DNS 泄露检测</a>
        <a
          href="/webrtc-leak-test/"
          aria-current={page === 'webrtc' ? 'page' : undefined}
          onclick={() => handleToolNavClick('webrtc')}
          data-umami-event={page === 'static' ? 'tool-open' : undefined}
          data-umami-event-from={page === 'static' ? 'static' : undefined}
          data-umami-event-to={page === 'static' ? 'webrtc' : undefined}
          data-umami-event-source={page === 'static' ? 'nav' : undefined}
        >WebRTC 泄露检测</a>
        <a
          href="/ipv6-leak-test/"
          aria-current={page === 'ipv6' ? 'page' : undefined}
          onclick={() => handleToolNavClick('ipv6')}
          data-umami-event={page === 'static' ? 'tool-open' : undefined}
          data-umami-event-from={page === 'static' ? 'static' : undefined}
          data-umami-event-to={page === 'static' ? 'ipv6' : undefined}
          data-umami-event-source={page === 'static' ? 'nav' : undefined}
        >IPv6 泄露检测</a>
      </nav>

      <div class="report-actions">
        {#if page !== "static"}
          <span class="status-pill text-xs font-mono text-(--slate-500) whitespace-nowrap" role="status" data-nosnippet>
            <span aria-hidden="true" class="dot" style:background={statusColor} style:--_soft="transparent"></span>
            {statusText}
          </span>
        {/if}

        <div class="icon-actions">
          {#if page !== "static" && (ready || !isScanning || page === "dns" || page === "webrtc" || page === "ipv6")}
            <button type="button" onclick={onRescan} disabled={isScanning} class="hbtn" title="重新检测" aria-label="重新检测">
              <i class="fa-solid fa-rotate-right text-sm" aria-hidden="true"></i>
            </button>
            {#if canShare && (ready || page === "dns" || page === "webrtc" || page === "ipv6")}
              <div class="share-wrap" bind:this={shareWrap}>
                <button
                  type="button"
                  onclick={toggleShareMenu}
                  disabled={shareDisabled}
                  class="hbtn"
                  class:is-open={shareOpen}
                  title="分享"
                  aria-label="分享"
                  aria-haspopup={onCopyText && onScreenshot ? 'menu' : undefined}
                  aria-expanded={onCopyText && onScreenshot ? shareOpen : undefined}
                >
                  <i class="fa-solid fa-share-nodes text-sm" aria-hidden="true"></i>
                </button>
                {#if shareOpen && onCopyText && onScreenshot}
                  <div class="share-menu" role="menu" aria-label="分享">
                    <button type="button" role="menuitem" class="share-item" onclick={runCopyText}>
                      <i class="fa-regular fa-copy share-icon" aria-hidden="true"></i>
                      <span>复制文字</span>
                    </button>
                    <button type="button" role="menuitem" class="share-item" onclick={runScreenshot}>
                      <i class="fa-regular fa-image share-icon" aria-hidden="true"></i>
                      <span>保存图片</span>
                    </button>
                  </div>
                {/if}
              </div>
            {/if}
          {/if}

          <button
            type="button"
            class="hbtn menu-btn"
            class:is-open={menuOpen}
            onclick={toggleNavMenu}
            title={menuOpen ? '关闭导航菜单' : '打开导航菜单'}
            aria-label={menuOpen ? '关闭导航菜单' : '打开导航菜单'}
            aria-expanded={menuOpen}
            aria-controls="site-nav"
            data-nav-toggle={page === 'static' ? 'static' : undefined}
          >
            <i class="fa-solid fa-bars text-sm menu-icon-open" aria-hidden="true"></i>
            <i class="fa-solid fa-xmark text-sm menu-icon-close" aria-hidden="true"></i>
          </button>
        </div>
      </div>
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
    max-width: 100%;
    gap: 0 0.5rem;
  }
  .status-pill {
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }
  .icon-actions { display: flex; align-items: center; column-gap: inherit; }
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
  .site-nav a[aria-current="page"] { color: var(--radar-400); font-weight: 600; }
  @media (hover: hover) {
    .site-nav a:hover { color: white; }
  }
  .site-nav a:focus-visible, .hbtn:focus-visible {
    outline: 2px solid var(--radar-400);
    outline-offset: 2px;
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
  .hbtn:hover:not(:disabled),
  .hbtn.is-open {
    color: var(--radar-400);
    background: var(--slate-700);
  }
  .hbtn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .menu-btn {
    display: none;
  }
  .menu-btn .menu-icon-close {
    display: none;
  }
  .menu-btn.is-open .menu-icon-open {
    display: none;
  }
  .menu-btn.is-open .menu-icon-close {
    display: inline-block;
  }

  @media (max-width: 62rem) {
    .header-row {
      grid-template-areas: "brand actions";
      grid-template-columns: minmax(0, 1fr) auto;
      min-height: 3.5rem;
    }
    .menu-btn {
      display: flex;
    }
    .site-nav {
      display: none;
      grid-area: auto;
      place-self: auto;
    }
    .site-nav.is-open {
      display: flex;
      position: absolute;
      top: 100%;
      left: 0;
      right: 0;
      width: 100%;
      z-index: 55;
      flex-direction: column;
      align-items: stretch;
      gap: 0.25rem;
      padding: 0.625rem 0.5rem 0.875rem;
      background: #060d1e;
      border-top: 1px solid rgba(148, 163, 184, 0.14);
      border-bottom: 1px solid rgba(148, 163, 184, 0.2);
      box-shadow: 0 20px 44px rgba(2, 6, 23, 0.85);
    }
    .site-nav.is-open a {
      min-height: 44px;
      padding: 0.6rem 0.5rem;
      border-radius: 0.55rem;
      font-size: 0.9375rem;
      color: var(--slate-200);
      transition: background-color 0.15s ease, color 0.15s ease;
    }
    .site-nav.is-open a:hover,
    .site-nav.is-open a:focus-visible {
      background: rgba(30, 41, 59, 0.78);
      color: #fff;
    }
    .site-nav.is-open a[aria-current="page"] {
      color: var(--radar-400);
      background: rgba(74, 222, 128, 0.08);
      font-weight: 600;
    }
  }

  @media (max-width: 30rem) {
    .status-pill {
      display: none;
    }
  }
  .share-wrap {
    position: relative;
  }
  .share-menu {
    position: absolute;
    right: 0;
    top: calc(100% + 0.5rem);
    z-index: 60;
    min-width: 8.5rem;
    max-width: calc(100vw - 1.5rem);
    padding: 0.3rem;
    border-radius: 0.65rem;
    border: 1px solid rgba(148, 163, 184, 0.18);
    background: rgba(15, 23, 42, 0.96);
    box-shadow: 0 14px 32px rgba(2, 6, 23, 0.6);
    backdrop-filter: blur(14px);
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .share-item {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    padding: 0.5rem 0.65rem;
    border-radius: 0.45rem;
    text-align: left;
    white-space: nowrap;
    font-size: 0.84rem;
    font-weight: 500;
    line-height: 1.25;
    color: var(--slate-200);
    transition: background-color 0.15s ease, color 0.15s ease;
    cursor: pointer;
  }
  .share-item:hover,
  .share-item:focus-visible {
    background: rgba(30, 41, 59, 0.85);
    color: #fff;
    outline: none;
  }
  .share-item:focus-visible {
    box-shadow: inset 0 0 0 1.5px var(--radar-400);
  }
  .share-icon {
    width: 1rem;
    text-align: center;
    color: var(--radar-400);
    font-size: 0.85rem;
    flex-shrink: 0;
  }
  @media (max-width: 23rem) {
    .report-actions { column-gap: 0.3rem; }
    .hbtn { width: 34px; height: 34px; }
    .header-wrap { padding-inline: 0.75rem; }
    .brand { gap: 0.35rem; }
    .brand > span:last-child { font-size: 1.05rem; }
  }
  @media (max-width: 20rem) {
    .brand > span:first-child { display: none; }
  }
</style>
