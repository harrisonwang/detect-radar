<script lang="ts">
  import { tick } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import { submitScanFeedback, type FeedbackCategory } from '../../lib/api';

  interface Props {
    scanId: string;
  }
  let { scanId }: Props = $props();

  const OPTIONS: { value: FeedbackCategory; label: string }[] = [
    { value: 'false_positive', label: '我是正常网络，被误判' },
    { value: 'missed_detection', label: '在用代理/VPN，但没检出' },
    { value: 'data_wrong', label: '位置/运营商/ASN 等数据不对' },
    { value: 'other', label: '其他' },
  ];

  let isOpen = $state(false);
  let submitting = $state(false);
  // 终态：提交成功后不再回到表单，重新打开弹层仍显示已收到
  let submitted = $state(false);
  let selected = $state<FeedbackCategory | null>(null);
  let note = $state('');
  let errorMsg = $state<string | null>(null);

  let triggerEl: HTMLButtonElement | undefined = $state();
  let dialogEl: HTMLDivElement | undefined = $state();

  async function openModal() {
    isOpen = true;
    errorMsg = null;
    await tick();
    dialogEl?.focus(); // 焦点移入弹层
  }

  // 取消按钮 / 点遮罩 / Esc 三路关闭；提交中一律禁止关闭
  function closeModal() {
    if (submitting) return;
    isOpen = false;
    errorMsg = null;
    triggerEl?.focus(); // 焦点归还触发入口
  }

  function onKeydown(e: KeyboardEvent) {
    if (isOpen && e.key === 'Escape') closeModal();
  }

  // 弹层打开期间锁定页面滚动
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
    errorMsg = null;
    try {
      await submitScanFeedback(scanId, selected, note.trim() || undefined);
      submitted = true;
    } catch (err) {
      errorMsg = (err as Error)?.message ?? '提交失败';
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- 低调入口：不与主判定争视觉 -->
<div class="text-center">
  <button
    bind:this={triggerEl}
    onclick={openModal}
    class="text-xs text-(--slate-500) hover:text-(--slate-300) transition-colors cursor-pointer inline-flex items-center gap-1.5"
  >
    <i class="fa-regular fa-flag"></i> 结果不对？帮我们校准
  </button>
</div>

{#if isOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <!-- 遮罩：半透明深空底 + 轻模糊，点击关闭（提交中除外） -->
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
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
      class="glass-card relative rounded-xl w-full max-w-md mx-4 p-6 focus:outline-none"
      transition:scale={{ duration: 180, start: 0.96 }}
    >
      {#if submitted}
        <!-- 终态：收到 -->
        <div class="text-center py-4">
          <i class="fa-solid fa-circle-check text-2xl" style="color:var(--sig)"></i>
          <p class="mt-3 text-sm text-(--slate-200) leading-relaxed">
            收到，谢谢。真实反馈是这个工具变准的唯一途径。
          </p>
          <button
            onclick={closeModal}
            class="mt-5 text-sm px-4 py-2 rounded-lg text-(--slate-400) hover:text-white transition-colors cursor-pointer"
          >
            关闭
          </button>
        </div>
      {:else}
        <header>
          <h3 id="feedback-modal-title" class="text-base font-semibold text-white tracking-wide">
            反馈：结果不对？
          </h3>
          <p class="mt-1 text-xs text-(--slate-500) leading-relaxed">
            选一类最接近的情况，可补充细节；反馈只用于校准判定规则。
          </p>
        </header>

        <div class="mt-4 space-y-2">
          {#each OPTIONS as opt}
            {@const active = selected === opt.value}
            <button
              type="button"
              onclick={() => (selected = opt.value)}
              class="w-full flex items-center justify-between gap-3 text-left px-3.5 py-2.5 rounded-lg border transition-colors cursor-pointer"
              style="border-color:{active ? 'var(--radar-500)' : 'var(--slate-800)'}; background:{active
                ? 'var(--sig-soft)'
                : 'transparent'}"
            >
              <span class="text-sm" style="color:{active ? 'var(--slate-200)' : 'var(--slate-400)'}">
                {opt.label}
              </span>
              {#if active}
                <i class="fa-solid fa-check text-xs shrink-0" style="color:var(--radar-400)"></i>
              {/if}
            </button>
          {/each}
        </div>

        <textarea
          bind:value={note}
          maxlength="500"
          rows="2"
          placeholder={'可选：具体哪里不对？例如“我在家用宽带，没开任何代理”'}
          class="mt-3 w-full text-sm bg-(--space-950)/60 border border-slate-800 rounded-lg px-3 py-2 text-(--slate-200) placeholder:text-(--slate-600) focus:outline-none focus:border-(--slate-600) resize-none transition-colors"
        ></textarea>
        <div class="mt-1 text-right text-[10px] font-mono text-(--slate-600) tnum">
          {note.length}/500
        </div>

        {#if errorMsg}
          <p class="mt-2 text-xs flex items-center gap-1.5" style="color:var(--leak)">
            <i class="fa-solid fa-triangle-exclamation"></i> 提交失败，请重试
          </p>
        {/if}

        <div class="mt-4 flex items-center justify-end gap-2">
          <button
            onclick={closeModal}
            disabled={submitting}
            class="text-sm px-3.5 py-2 rounded-lg text-(--slate-400) hover:text-white transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
          >
            取消
          </button>
          <button
            onclick={submit}
            disabled={!selected || submitting}
            class="text-sm font-medium px-5 py-2 rounded-lg transition-colors cursor-pointer inline-flex items-center gap-1.5 disabled:opacity-40 disabled:cursor-not-allowed"
            style="color:var(--space-950); background:var(--radar-400)"
          >
            {#if submitting}
              <i class="fa-solid fa-spinner fa-spin"></i> 提交中…
            {:else if errorMsg}
              重试
            {:else}
              提交
            {/if}
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
