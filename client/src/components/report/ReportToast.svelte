<script lang="ts">
  import { onDestroy } from 'svelte';

  let { suppressed = false }: { suppressed?: boolean } = $props();
  let message = $state('');
  let timer: ReturnType<typeof setTimeout> | undefined;

  export function dismiss() {
    clearTimeout(timer);
    message = '';
  }

  export function show(text: string) {
    clearTimeout(timer);
    message = text;
    timer = setTimeout(dismiss, 3000);
  }

  onDestroy(() => clearTimeout(timer));
</script>

{#if message && !suppressed}
  <div class="fixed bottom-6 right-6 z-50 reveal" role="status">
    <div class="bg-slate-900/95 border border-white/10 rounded-lg flex items-center gap-2.5 px-4 py-3 shadow-xl">
      <i class="fa-solid fa-circle-check" style="color:var(--sig)" aria-hidden="true"></i>
      <span class="text-sm text-white">{message}</span>
    </div>
  </div>
{/if}
