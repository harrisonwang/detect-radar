<script lang="ts">
  import { maskIP } from '../../lib/ipDisplay';

  interface Props {
    ip: string;
    revealed: boolean;
    onToggleReveal?: () => void;
    onCopyFeedback?: (message: string) => void;
  }

  let { ip, revealed, onToggleReveal, onCopyFeedback }: Props = $props();
  let address = $derived(revealed ? ip || '—' : maskIP(ip));
  let action = $derived(revealed ? '点击脱敏' : '点击显示完整 IP');

  async function copyIP() {
    try {
      await navigator.clipboard.writeText(ip);
      onCopyFeedback?.('IP 已复制');
    } catch {
      onCopyFeedback?.('复制失败，请检查浏览器权限');
    }
  }
</script>

<div class="flex min-w-0 items-center gap-2.5">
  <button
    type="button"
    class="min-w-0 break-all text-left font-mono font-semibold text-white hover:text-(--radar-400) transition-colors cursor-pointer"
    class:text-base={revealed && ip.includes(':')}
    class:text-2xl={!revealed || !ip.includes(':')}
    onclick={onToggleReveal}
    title={action}
    aria-label={`${action}，${address}`}
    data-ip-display
  >{address}</button>
  {#if ip && onCopyFeedback}
    <button type="button" class="shrink-0 text-(--slate-500) hover:text-(--radar-400) transition-colors cursor-pointer"
      onclick={copyIP} title="复制完整 IP" aria-label="复制">
      <i class="fa-regular fa-copy text-sm" aria-hidden="true"></i>
    </button>
  {/if}
</div>
