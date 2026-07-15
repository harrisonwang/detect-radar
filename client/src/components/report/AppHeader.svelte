<script lang="ts">
  interface Props {
    onScreenshot?: () => void;
    onShare?: () => void;
    isScanning?: boolean;
    ready?: boolean;
  }
  let { onScreenshot, onShare, isScanning = false, ready = false }: Props = $props();
</script>

<nav class="border-b border-slate-800/60 bg-space-950/80 sticky top-0 z-50 backdrop-blur-md">
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="flex items-center justify-between h-16">
      <a href="/" class="flex items-center gap-3 group">
        <span class="relative w-8 h-8 flex items-center justify-center">
          <i class="fa-solid fa-crosshairs text-(--radar-400) text-xl group-hover:rotate-180 transition-transform duration-700"></i>
          <span class="absolute inset-0 bg-(--radar-400)/2 rounded-full animate-ping-slow"></span>
        </span>
        <span class="text-xl font-bold tracking-tight text-white font-mono">Detect<span class="text-(--radar-400)">Radar</span></span>
      </a>

      <div class="flex items-center gap-2">
        <span class="hidden sm:flex items-center gap-1.5 text-xs font-mono text-(--slate-500) mr-1">
          <span class="dot" style="background:{isScanning ? 'var(--warn)' : ready ? 'var(--sig)' : 'var(--unknown)'}; --_soft:transparent"></span>
          {isScanning ? '检测中' : ready ? '在线' : '待命'}
        </span>
        {#if ready}
          <button onclick={onScreenshot} class="hbtn" title="生成分享图（IP 自动脱敏）" aria-label="生成分享图">
            <i class="fa-solid fa-camera text-sm"></i>
          </button>
          <!-- <button onclick={onShare} class="hbtn" title="分享" aria-label="分享">
            <i class="fa-solid fa-share-nodes text-sm"></i>
          </button> -->
        {/if}
      </div>
    </div>
  </div>
</nav>

<style>
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
</style>
