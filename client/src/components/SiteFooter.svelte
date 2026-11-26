<script lang="ts">
  import { getMessages, localePath, type Locale } from '../i18n';

  interface Props {
    locale?: Locale;
  }
  let { locale = 'zh-CN' }: Props = $props();
  let m = $derived(getMessages(locale));
</script>

<footer class="border-t border-slate-800 py-8 relative z-10">
  <div class="max-w-7xl mx-auto px-4 flex flex-col md:flex-row justify-between items-center text-(--slate-400) text-sm gap-3">
    <p>© 2026 DetectRadar.com</p>
    <nav aria-label={m.footer.ariaLabel} class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2">
      <a href={localePath('/macos/', locale)} class="inline-flex items-center gap-1.5 hover:text-white transition">
        <i class="fa-brands fa-apple text-[15px] leading-none -translate-y-[0.5px]" aria-hidden="true"></i>
        <span>{m.footer.macos}</span>
      </a>
      <span aria-hidden="true" class="hidden sm:inline-block h-3 w-px bg-slate-700/80"></span>
      <a href={localePath('/about/', locale)} class="hover:text-white transition">{m.footer.about}</a>
      <a href={localePath('/privacy/', locale)} class="hover:text-white transition">{m.footer.privacy}</a>
      <a href={localePath('/terms/', locale)} class="hover:text-white transition">{m.footer.terms}</a>
      <a href={localePath('/contact/', locale)} class="hover:text-white transition">{m.footer.contact}</a>
    </nav>
  </div>
</footer>
