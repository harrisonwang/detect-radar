<script lang="ts">
  import { getMessages, type Locale } from '../../i18n';
  import { maskIP } from '../../lib/ipDisplay';

  interface Props {
    locale?: Locale;
    ip: string;
    revealed: boolean;
    onToggleReveal?: () => void;
    onCopyFeedback?: (message: string) => void;
  }

  let { locale = 'zh-CN', ip, revealed, onToggleReveal, onCopyFeedback }: Props = $props();
  let c = $derived(getMessages(locale).common);
  let address = $derived(revealed ? ip || '—' : maskIP(ip));
  let action = $derived(revealed ? c.ipMask : c.ipReveal);

  async function copyIP() {
    try {
      await navigator.clipboard.writeText(ip);
      onCopyFeedback?.(c.ipCopied);
    } catch {
      onCopyFeedback?.(c.copyFailedShort);
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
    aria-label={locale === 'en' ? `${action}, ${address}` : `${action}，${address}`}
    data-ip-display
  >{address}</button>
  {#if ip && onCopyFeedback}
    <button
      type="button"
      class="shrink-0 text-(--slate-500) hover:text-(--radar-400) transition-colors cursor-pointer"
      onclick={copyIP}
      title={c.copyFullIP}
      aria-label={c.copyAction}
    >
      <i class="fa-regular fa-copy text-sm" aria-hidden="true"></i>
    </button>
  {/if}
</div>
