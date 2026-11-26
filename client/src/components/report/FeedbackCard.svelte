<script lang="ts">
  import { tick } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import { getMessages, type Locale } from '../../i18n';
  import { submitScanFeedback, type FeedbackCategory } from '../../lib/api';

  interface Props {
    locale?: Locale;
    scanId: string;
  }
  let { locale = 'zh-CN', scanId }: Props = $props();

  let fb = $derived(getMessages(locale).home.feedback);

  let options = $derived<{ value: FeedbackCategory; label: string }[]>([
    { value: 'false_positive', label: fb.options.false_positive },
    { value: 'missed_detection', label: fb.options.missed_detection },
    { value: 'data_wrong', label: fb.options.data_wrong },
    { value: 'other', label: fb.options.other },
  ]);

  let isOpen = $state(false);
  let submitting = $state(false);
  let submitted = $state(false);
  let selected = $state<FeedbackCategory | null>(null);
  let note = $state('');
  let replyEmail = $state('');
  let submitFailed = $state(false);

  let triggerEl: HTMLButtonElement | undefined = $state();
  let dialogEl: HTMLDivElement | undefined = $state();

  async function openModal() {
    isOpen = true;
    submitFailed = false;
    await tick();
    dialogEl?.focus();
  }

  function closeModal() {
    if (submitting) return;
    isOpen = false;
    submitFailed = false;
    triggerEl?.focus();
  }

  function onKeydown(e: KeyboardEvent) {
    if (isOpen && e.key === 'Escape') closeModal();
  }

  $effect(() => {
    if (isOpen) {
      const prev = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
      return () => {
        document.body.style.overflow = prev;
      };
    }
  });

  async function submit() {
    if (!selected || submitting) return;
    submitting = true;
    submitFailed = false;
    try {
      await submitScanFeedback(scanId, selected, note.trim() || undefined, replyEmail.trim() || undefined, locale);
      submitted = true;
    } catch {
      submitFailed = true;
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="text-center">
  <button
    bind:this={triggerEl}
    onclick={openModal}
    class="text-xs text-(--slate-500) hover:text-(--slate-300) transition-colors cursor-pointer inline-flex items-center gap-1.5"
  >
    <i class="fa-regular fa-flag"></i> {fb.trigger}
  </button>
</div>

{#if isOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <div
      class="absolute inset-0 backdrop-blur-sm"
      style="background:rgba(2, 6, 23, 0.72)"
      transition:fade={{ duration: 150 }}
      onclick={closeModal}
    ></div>

    <div
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="feedback-modal-title"
      tabindex="-1"
      class="relative z-10 w-full max-w-md max-h-[calc(100dvh-2rem)] overflow-y-auto mx-4 rounded-xl bg-[#111822] border border-white/10 p-5 text-(--slate-200) shadow-2xl focus:outline-none"
      transition:scale={{ duration: 180, start: 0.96 }}
    >
      {#if submitted}
        <div class="text-center py-4">
          <i class="fa-solid fa-circle-check text-2xl" style="color:var(--sig)"></i>
          <p class="mt-3 text-sm text-(--slate-200) leading-relaxed">
            {fb.received}
          </p>
          <button
            onclick={closeModal}
            class="mt-4 text-xs px-4 py-1.5 rounded-lg text-(--slate-400) hover:text-white transition-colors cursor-pointer"
          >
            {fb.close}
          </button>
        </div>
      {:else}
        <form onsubmit={(event) => { event.preventDefault(); submit(); }}>
          <div class="flex items-center justify-between mb-4">
            <p id="feedback-modal-title" class="text-base font-semibold text-white">
              {fb.title}
            </p>
            <button
              type="button"
              onclick={closeModal}
              disabled={submitting}
              aria-label={fb.close}
              class="text-(--slate-500) hover:text-(--slate-300) transition-colors cursor-pointer"
            >
              ✕
            </button>
          </div>

          <div class="grid grid-cols-2 gap-2.5 mb-3">
            {#each options as opt}
              {@const active = selected === opt.value}
              <button
                type="button"
                onclick={() => (selected = opt.value)}
                class="px-3 py-2.5 rounded-lg border text-xs text-left transition cursor-pointer {active
                  ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300 font-medium'
                  : 'border-white/10 bg-white/[0.02] hover:bg-white/[0.05] text-(--slate-300)'}"
              >
                {opt.label}
              </button>
            {/each}
          </div>

          <div class="rounded-lg bg-black/25 border border-white/10 overflow-hidden focus-within:border-emerald-500/45 transition-colors">
            <textarea
              bind:value={note}
              maxlength="500"
              rows="3"
              placeholder={fb.notePlaceholder}
              class="w-full bg-transparent px-3 py-2.5 text-xs leading-relaxed text-(--slate-200) placeholder:text-(--slate-500) focus:outline-none resize-none block"
            ></textarea>
            <div class="border-t border-white/[0.06] bg-white/[0.015] px-3 py-2 flex items-center gap-2">
              <i class="fa-regular fa-envelope text-xs text-(--slate-500) shrink-0"></i>
              <input
                type="email"
                autocomplete="email"
                maxlength="254"
                bind:value={replyEmail}
                placeholder={fb.emailPlaceholder}
                class="w-full bg-transparent text-xs text-(--slate-200) placeholder:text-(--slate-500) focus:outline-none"
              />
            </div>
          </div>

          {#if submitFailed}
            <p role="alert" class="mt-2 text-xs flex items-center gap-1.5" style="color:var(--leak)">
              <i class="fa-solid fa-triangle-exclamation"></i> {fb.failed}
            </p>
          {/if}

          <div class="mt-4 flex items-center justify-end gap-2">
            <button
              type="button"
              onclick={closeModal}
              disabled={submitting}
              class="px-3 py-1.5 text-xs text-(--slate-400) hover:text-(--slate-200) transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {fb.cancel}
            </button>
            <button
              type="submit"
              disabled={!selected || submitting}
              class="px-4 py-1.5 rounded-lg bg-emerald-500 hover:bg-emerald-400 text-slate-950 text-xs font-semibold transition-colors cursor-pointer inline-flex items-center gap-1.5 disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {#if submitting}
                <i class="fa-solid fa-spinner fa-spin"></i> {fb.submitting}
              {:else if submitFailed}
                {fb.retry}
              {:else}
                {fb.submit}
              {/if}
            </button>
          </div>
        </form>
      {/if}
    </div>
  </div>
{/if}
