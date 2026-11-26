<script lang="ts">
  import { onDestroy } from 'svelte';

  let { suppressed = false }: { suppressed?: boolean } = $props();
  let message = $state('');
  let tone = $state<'ok' | 'error'>('ok');
  let timer: ReturnType<typeof setTimeout> | undefined;

  export function dismiss() {
    clearTimeout(timer);
    message = '';
  }

  export function show(text: string, nextTone: 'ok' | 'error' = 'ok') {
    clearTimeout(timer);
    message = text;
    tone = nextTone;
    timer = setTimeout(dismiss, 3000);
  }

  onDestroy(() => clearTimeout(timer));
</script>

{#if message && !suppressed}
  <div class="fixed bottom-6 inset-x-4 sm:left-auto sm:right-6 sm:max-w-md z-50 reveal" role="status">
    <div class="bg-slate-900/95 border border-white/10 rounded-lg flex items-center gap-2.5 px-4 py-3 shadow-xl">
      <i
        class="fa-solid {tone === 'error' ? 'fa-circle-exclamation' : 'fa-circle-check'} shrink-0"
        style="color:{tone === 'error' ? 'var(--leak)' : 'var(--sig)'}"
        aria-hidden="true"
      ></i>
      <span class="text-sm text-white break-words min-w-0">{message}</span>
    </div>
  </div>
{/if}
