<script lang="ts">
  import { onMount } from 'svelte';

  import AppHeader from './report/AppHeader.svelte';
  import RadarCard from './report/RadarCard.svelte';
  import DiagnosisCard from './report/DiagnosisCard.svelte';
  import IdentityCard from './report/IdentityCard.svelte';
  import ReportToast from './report/ReportToast.svelte';
  import ReadoutCard from './report/ReadoutCard.svelte';
  import FeedbackCard from './report/FeedbackCard.svelte';

  import { getMessages, type Locale } from '../i18n';
  import { collectFingerprints } from '../lib/collectors/fingerprint';
  import { collectEnvironment } from '../lib/collectors/environment';
  import { collectLeaks, collectDNSLeak } from '../lib/collectors/leaks';
  import { collectRTT } from '../lib/collectors/network/rtt';
  import { collectFingerprintAudit } from '../lib/collectors/fingerprintAudit';
  import type { AllCollectorResults } from '../lib/collectors/types';
  import * as api from '../lib/api';
  import { trackEvent } from '../lib/analytics';
  import { isAutomatedCrawler } from '../lib/crawler';
  import { buildReport, type ReportModel, type IdentityReadout } from '../lib/report';
  import { renderShareCard } from '../lib/shareCard';
  import { copyShareText, formatHomeShareText } from '../lib/shareSummary';
  import type { ScanRequest, ScanScenario } from '../lib/types';

  interface Props {
    locale?: Locale;
  }
  let { locale = 'zh-CN' }: Props = $props();

  let m = $derived(getMessages(locale));

  interface ScanSignals {
    collectors: AllCollectorResults;
    fpjs: Awaited<ReturnType<typeof collectFingerprintAudit>>;
  }

  let emptyIdentity = $derived<IdentityReadout>({
    ip: '',
    masked: '···.···.···.···',
    geo: '—',
    countryCode: '',
    asn: '—',
    org: '—',
    ipType: 'unknown',
    status: 'loading',
    usageLabel: m.common.scanning,
    state: 'unknown',
    flags: [],
    fraudScore: 0,
    blacklist: [],
    openPorts: [],
    exposure: [],
    present: false,
  });

  const SCENARIO_KEY = 'detectradar:scenario';

  type ScanStage = 'identity' | 'consistency' | 'leak' | 'environment' | 'analysis' | 'complete';
  type DimensionKey = Exclude<ScanStage, 'analysis' | 'complete'>;
  const STAGE_ORDER: ScanStage[] = ['identity', 'consistency', 'leak', 'environment', 'analysis'];

  let placeholderGroups = $derived([
    { key: 'identity' as const, label: m.home.groups.identity, score: 0, state: 'unknown' as const, passed: 0, total: 1 },
    { key: 'consistency' as const, label: m.home.groups.consistency, score: 0, state: 'unknown' as const, passed: 0, total: 1 },
    { key: 'leak' as const, label: m.home.groups.leak, score: 0, state: 'unknown' as const, passed: 0, total: 1 },
    { key: 'environment' as const, label: m.home.groups.environment, score: 0, state: 'unknown' as const, passed: 0, total: 1 },
  ]);

  let report = $state<ReportModel | null>(null);
  let scenario = $state<ScanScenario>('strict');
  let scenarioSwitchToken = 0;
  let isScanning = $state(true);
  let scanError = $state<string | null>(null);
  let dnsPending = $state(true);
  let ipRevealed = $state(true);
  let scanStage = $state<ScanStage>('identity');
  let completedStages = $state<ScanStage[]>([]);
  let revealedKeys = $state<DimensionKey[]>([]);
  let revealTimers: number[] = [];
  let lastSignals: ScanSignals | null = null;

  let scanStageLabel = $derived(m.home.stages[scanStage] ?? m.home.stages.complete);
  let displayStageLabel = $derived(
    isScanning
      ? scanStage === 'analysis' && dnsPending && report
        ? m.home.stages.leak
        : scanStageLabel
      : dnsPending
        ? m.home.stages.leak
        : '',
  );
  let activeDimension = $derived(
    isScanning
      ? scanStage === 'analysis' && dnsPending && report
        ? 'leak'
        : scanStage === 'analysis'
          ? null
          : scanStage
      : dnsPending
        ? 'leak'
        : null,
  );

  function riskText(level: string): string {
    return m.home.riskLevel[level as keyof typeof m.home.riskLevel] ?? level;
  }

  function resetScanProgress() {
    revealTimers.forEach((timer) => window.clearTimeout(timer));
    revealTimers = [];
    completedStages = [];
    revealedKeys = [];
    scanStage = 'identity';
  }

  function completeStage(stage: ScanStage) {
    if (completedStages.includes(stage)) return;
    completedStages = [...completedStages, stage];
    const next = STAGE_ORDER.find((key) => !completedStages.includes(key));
    if (next) scanStage = next;
  }

  function revealDimension(key: DimensionKey) {
    if (!report || revealedKeys.includes(key)) return;
    revealedKeys = [...revealedKeys, key];
    scanStage = key;
  }

  function queueReadyReveals() {
    const order: DimensionKey[] = ['identity', 'consistency', 'environment'];
    order.forEach((key, index) => {
      if (!completedStages.includes(key)) return;
      const timer = window.setTimeout(() => revealDimension(key), 120 + index * 170);
      revealTimers = [...revealTimers, timer];
    });
    const finishTimer = window.setTimeout(() => {
      if (isScanning && dnsPending) scanStage = 'analysis';
    }, 120 + order.length * 170);
    revealTimers = [...revealTimers, finishTimer];
  }

  async function collectSignals(onStageComplete?: (stage: DimensionKey) => void): Promise<ScanSignals> {
    const fingerprintPromise = collectFingerprints();
    const environmentPromise = collectEnvironment().then((value) => {
      onStageComplete?.('consistency');
      return value;
    });
    const leaksPromise = collectLeaks().then((value) => {
      onStageComplete?.('leak');
      return value;
    });
    const fpjsPromise = collectFingerprintAudit();
    const environmentReady = Promise.all([fingerprintPromise, fpjsPromise]).then(() => {
      onStageComplete?.('environment');
    });

    const [fingerprint, environment, leaks, fpjs] = await Promise.all([
      fingerprintPromise,
      environmentPromise,
      leaksPromise,
      fpjsPromise,
    ]);
    await environmentReady;
    const rtt = await collectRTT();
    return { collectors: { fingerprint, environment, leaks, network: { rtt } }, fpjs };
  }

  function buildRequest(s: ScanSignals): ScanRequest {
    return {
      scan_id: crypto?.randomUUID ? crypto.randomUUID() : `scan_${Date.now()}`,
      scenario,
      fingerprint: s.collectors.fingerprint as Record<string, unknown>,
      environment: s.collectors.environment as Record<string, unknown>,
      leaks: s.collectors.leaks as Record<string, unknown>,
      fpjs_audit: s.fpjs,
      network: { rtt: s.collectors.network?.rtt },
    };
  }

  function friendlyError(err: unknown): string {
    const e = m.home.errors;
    if (err instanceof api.ApiError) {
      if (err.status === 0) return e.network;
      if (err.status === 429) return e.limited;
      if (err.status >= 500) return e.server;
      return e.rejected;
    }
    return e.generic;
  }

  async function runScan(mode: 'auto' | 'manual' | 'retest' = 'retest') {
    if (mode === 'retest') trackEvent('scan-retest', { page: 'home' });
    else trackEvent('scan-start', { page: 'home', trigger: mode });
    isScanning = true;
    scanError = null;
    dnsPending = true;
    report = null;
    resetScanProgress();
    try {
      const signals = await collectSignals(completeStage);
      lastSignals = signals;
      const resp = await api.createScan(buildRequest(signals), locale);
      const synced =
        resp.scenario && resp.scenario !== scenario
          ? await api.getScan(resp.scan_id, scenario, locale).catch(() => resp)
          : resp;
      report = buildReport(synced, signals.collectors, locale);
      completeStage('identity');
      queueReadyReveals();

      await enrichDNS();
      dnsPending = false;
      revealDimension('leak');
      await new Promise<void>((resolve) => window.setTimeout(resolve, 440));
      completeStage('analysis');
      scanStage = 'complete';
      isScanning = false;
    } catch (err) {
      const msg = friendlyError(err);
      scanError = msg;
      report = null;
      dnsPending = false;
      showToast(msg);
      isScanning = false;
      scanStage = 'complete';
    }
  }

  async function enrichDNS() {
    try {
      const scanId = report?.meta.scanId;
      const dns = await collectDNSLeak(scanId);
      if (!report || !scanId || dns.status !== 'success' || !dns.data || dns.data.resolvers.length === 0) return;
      const targetScenario = scenario;
      let updated = await api.getScan(scanId, targetScenario, locale);
      if (scenario !== targetScenario) {
        updated = await api.getScan(scanId, scenario, locale);
      }
      if (report?.meta.scanId === scanId && lastSignals) {
        report = buildReport(updated, lastSignals.collectors, locale);
      }
    } catch {
    }
  }

  async function selectScenario(next: ScanScenario) {
    if (scenario === next) return;
    scenario = next;
    try {
      window.localStorage.setItem(SCENARIO_KEY, next);
    } catch {
    }
    trackEvent('scenario-switch', { to: next });

    const currentId = report?.meta.scanId;
    const signals = lastSignals;
    if (!currentId || !signals) return;

    const token = ++scenarioSwitchToken;
    try {
      const updated = await api.getScan(currentId, next, locale);
      if (token !== scenarioSwitchToken || report?.meta.scanId !== currentId) return;
      report = buildReport(updated, signals.collectors, locale);
    } catch (err) {
      if (token !== scenarioSwitchToken) return;
      if (err instanceof api.ApiError && err.status === 404) {
        runScan('retest');
      }
    }
  }

  async function handleCopyPromo(code: string) {
    try {
      await navigator.clipboard.writeText(code);
      showToast(m.home.diagnosis.promoCopied(code));
    } catch {
      showToast(m.common.copyFailedShort);
    }
  }

  function toggleReveal() {
    ipRevealed = !ipRevealed;
  }

  onMount(() => {
    try {
      if (window.localStorage.getItem(SCENARIO_KEY) === 'daily') {
        scenario = 'daily';
      }
    } catch {
    }
    if (isAutomatedCrawler()) {
      isScanning = false;
      dnsPending = false;
      scanStage = 'complete';
      return;
    }
    runScan('auto');
    const onPageShow = (e: PageTransitionEvent) => {
      if (e.persisted) runScan('auto');
    };
    window.addEventListener('pageshow', onPageShow);
    return () => {
      revealTimers.forEach((timer) => window.clearTimeout(timer));
      window.removeEventListener('pageshow', onPageShow);
    };
  });

  let toast: ReturnType<typeof ReportToast> | undefined = $state();
  let isCapturing = $state(false);
  let captureProgress = $state(0);
  let captureTimer: number | null = null;

  function showToast(message: string) {
    toast?.show(message);
  }

  function startCaptureProgress() {
    captureProgress = 0;
    if (captureTimer) window.clearInterval(captureTimer);
    captureTimer = window.setInterval(() => {
      captureProgress = Math.min(captureProgress + Math.max(1, (100 - captureProgress) * 0.12), 90);
    }, 180);
  }
  function stopCaptureProgress() {
    if (captureTimer) {
      window.clearInterval(captureTimer);
      captureTimer = null;
    }
  }

  async function handleScreenshot() {
    if (!report) return;
    try {
      isCapturing = true;
      startCaptureProgress();
      toast?.dismiss();
      const blob = await renderShareCard(report, {
        riskLabel: riskText(report.riskLevel),
        locale,
      });
      captureProgress = 100;
      let clipboardOk = false;
      try {
        await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
        clipboardOk = true;
      } catch {
      }
      downloadScreenshot(blob);
      trackEvent('share-save-image', { page: 'home' });
      showToast(clipboardOk ? m.common.imageCopiedAndSaved : m.common.imageSavedOnly);
    } catch (error) {
      showToast(m.common.imageFailed((error as Error).message));
    } finally {
      isCapturing = false;
      stopCaptureProgress();
    }
  }

  function downloadScreenshot(blob: Blob) {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `detectradar-${new Date().toISOString().replace(/[:.]/g, '-')}.png`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function handleCopyText() {
    if (!report || isScanning) return;
    trackEvent('share-copy-text', { page: 'home' });
    showToast(await copyShareText(formatHomeShareText(report, locale), locale));
  }
</script>

<AppHeader
  {locale}
  onCopyText={handleCopyText}
  onScreenshot={handleScreenshot}
  onRescan={() => runScan('retest')}
  isScanning={isScanning}
  ready={!!report && !isScanning}
/>

<main class="report-main flex-1 pt-6 sm:pt-10">
  <div class="mb-5 sm:mb-8 flex flex-col gap-3.5 sm:flex-row sm:items-end sm:justify-between min-w-0">
    <div class="min-w-0">
      <h1 class="report-title text-2xl sm:text-3xl md:text-4xl mb-1.5 sm:mb-2">
        {m.home.titlePrefix}<span class="text-(--radar-400)">{m.home.titleAccent}</span>
      </h1>
      <p class="text-sm text-(--slate-400) leading-relaxed">{m.home.scenarios[scenario].subtitle}</p>
    </div>

    <div
      class="grid grid-cols-2 w-full sm:inline-flex sm:w-auto items-center gap-1 rounded-lg border border-white/10 bg-slate-900/80 p-1 shrink-0"
      role="radiogroup"
      aria-label={m.home.scenarioAria}
      data-nosnippet
    >
      {#each (['strict', 'daily'] as const) as mode}
        {@const active = scenario === mode}
        <button
          type="button"
          role="radio"
          aria-checked={active}
          onclick={() => selectScenario(mode)}
          class="px-3.5 py-1.5 rounded-md text-xs font-medium text-center whitespace-nowrap transition-colors cursor-pointer {active
            ? 'bg-slate-800 text-white shadow-sm ring-1 ring-white/10'
            : 'text-(--slate-400) hover:text-slate-200'}"
        >{m.home.scenarios[mode].label}</button>
      {/each}
    </div>
  </div>

  <div data-nosnippet>
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 mb-6">
      <div class="lg:col-span-4">
        <RadarCard
          {locale}
          groups={report?.groups ?? placeholderGroups}
          verdictState={report?.verdict.state ?? 'unknown'}
          riskLabel={report ? riskText(report.riskLevel) : '—'}
          isScanning={isScanning}
          pendingKeys={dnsPending ? ['leak'] : []}
          settledKeys={revealedKeys}
          activeKey={activeDimension}
          scanStageLabel={displayStageLabel}
        />
      </div>
      <div class="lg:col-span-8">
        <DiagnosisCard
          {locale}
          report={report}
          isScanning={isScanning}
          active={activeDimension === 'identity'}
          onCopyPromo={handleCopyPromo}
          onStart={() => runScan('manual')}
        />
      </div>
    </div>

    {#if scanError}
      <div class="glass-card rounded-xl p-8 text-center">
        <i class="fa-solid fa-plug-circle-xmark text-2xl mb-3" style="color:var(--leak)"></i>
        <p class="text-[15px] text-white">{m.home.errors.incomplete}</p>
        <p class="mt-1.5 text-xs text-(--slate-500) max-w-md mx-auto">{scanError}</p>
        <button
          onclick={() => runScan('retest')}
          class="mt-5 text-sm px-4 py-2 rounded-lg transition-colors cursor-pointer"
          style="color:var(--radar-400); background:var(--sig-soft)"
        >{m.common.rescan}</button>
      </div>
    {:else}
      <div class="mb-6">
        <IdentityCard
          {locale}
          identity={report?.identity ?? emptyIdentity}
          rtt={report?.rtt}
          isScanning={isScanning}
          idle={!isScanning && !report}
          revealed={ipRevealed}
          settled={revealedKeys.includes('identity')}
          active={activeDimension === 'identity'}
          onToggleReveal={toggleReveal}
          onCopyFeedback={showToast}
        />
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <ReadoutCard
          {locale}
          title={m.home.groups.consistency}
          icon="fa-globe"
          rows={report?.consistency ?? []}
          state={report?.groups[1]?.state ?? 'unknown'}
          referenceOnly={scenario === 'daily'}
          isScanning={isScanning}
          settled={revealedKeys.includes('consistency')}
          active={activeDimension === 'consistency'}
        />
        <ReadoutCard
          {locale}
          title={m.home.groups.leak}
          icon="fa-shield-halved"
          rows={report?.leaks ?? []}
          state={report?.groups[2]?.state ?? 'unknown'}
          isScanning={isScanning}
          settled={revealedKeys.includes('leak')}
          active={activeDimension === 'leak'}
        />
        <ReadoutCard
          {locale}
          title={m.home.groups.environment}
          icon="fa-fingerprint"
          rows={report?.environment ?? []}
          state={report?.groups[3]?.state ?? 'unknown'}
          isScanning={isScanning}
          settled={revealedKeys.includes('environment')}
          active={activeDimension === 'environment'}
        />
      </div>

      {#if report && !isScanning}
        <div class="mt-8">
          <FeedbackCard {locale} scanId={report.meta.scanId} />
        </div>
      {/if}
    {/if}
  </div>
</main>

<ReportToast bind:this={toast} suppressed={isCapturing} />

{#if isCapturing}
  <div class="fixed inset-0 z-50 pointer-events-none">
    <div class="absolute bottom-8 left-1/2 -translate-x-1/2 w-[320px]">
      <div class="glass-card rounded-lg px-4 py-3">
        <div class="text-xs text-(--slate-300) font-mono">{m.home.captureProgress(Math.round(captureProgress))}</div>
        <div class="mt-2 h-1.5 bg-slate-800 overflow-hidden rounded">
          <div class="h-full transition-all duration-200" style="width:{captureProgress}%; background:var(--radar-400)"></div>
        </div>
      </div>
    </div>
  </div>
{/if}
