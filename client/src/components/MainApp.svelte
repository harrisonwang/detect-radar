<script lang="ts">
  import { onMount } from 'svelte';

  import AppHeader from './report/AppHeader.svelte';
  import RadarCard from './report/RadarCard.svelte';
  import DiagnosisCard from './report/DiagnosisCard.svelte';
  import IdentityCard from './report/IdentityCard.svelte';
  import ReportToast from './report/ReportToast.svelte';
  import ReadoutCard from './report/ReadoutCard.svelte';
  import FeedbackCard from './report/FeedbackCard.svelte';

  import { collectFingerprints } from '../lib/collectors/fingerprint';
  import { collectEnvironment } from '../lib/collectors/environment';
  import { collectDeviceSignals, type DeviceSignals } from '../lib/collectors/environment/device';
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

  interface ScanSignals {
    collectors: AllCollectorResults;
    device: DeviceSignals;
    fpjs: Awaited<ReturnType<typeof collectFingerprintAudit>>;
  }

  const EMPTY_IDENTITY: IdentityReadout = {
    ip: '', masked: '···.···.···.···', geo: '—', countryCode: '', asn: '—', org: '—',
    usageLabel: '检测中', state: 'unknown', flags: [], fraudScore: 0,
    blacklist: [], openPorts: [], exposure: [], present: false,
  };

  const RISK_LABEL: Record<string, string> = {
    safe: '很低', low: '较低', medium: '中等', high: '偏高', critical: '严重',
  };
  const SCENARIO_KEY = 'detectradar:scenario';
  const SCENARIO_META: Record<ScanScenario, { label: string; subtitle: string }> = {
    strict: { label: '严格模式', subtitle: '核对出口 IP、时区语言、网络泄露与浏览器指纹' },
    daily: { label: '日常模式', subtitle: '聚焦网络泄露与高危 IP，时区、语言与普通机房仅作展示' },
  };

  type ScanStage = 'identity' | 'consistency' | 'leak' | 'environment' | 'analysis' | 'complete';
  type DimensionKey = Exclude<ScanStage, 'analysis' | 'complete'>;
  const SCAN_STAGES: { key: ScanStage; label: string }[] = [
    { key: 'identity', label: '正在获取出口 IP' },
    { key: 'consistency', label: '正在比对时区与语言' },
    { key: 'leak', label: '正在检查 WebRTC、DNS、IPv6' },
    { key: 'environment', label: '正在分析环境指纹' },
    { key: 'analysis', label: '正在汇总检测结果' },
  ];

  // 检测中的占位四轴（雷达在扫描态仍显示四轴骨架）
  const PLACEHOLDER_GROUPS = [
    { key: 'identity', label: '网络身份', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
    { key: 'consistency', label: '环境一致性', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
    { key: 'leak', label: 'DNS/IP 泄露', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
    { key: 'environment', label: '环境指纹', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
  ] as const;

  let report = $state<ReportModel | null>(null);
  let scenario = $state<ScanScenario>('strict');
  let scenarioSwitchToken = 0;
  let isScanning = $state(true);
  let scanError = $state<string | null>(null);
  // DNS 泄露走独立慢路径（enrichDNS 约 4s），未定前雷达该轴显示待定而非中间值
  let dnsPending = $state(true);
  let ipRevealed = $state(true);
  let scanStage = $state<ScanStage>('identity');
  let completedStages = $state<ScanStage[]>([]);
  let revealedKeys = $state<DimensionKey[]>([]);
  let revealTimers: number[] = [];
  let lastSignals: ScanSignals | null = null;

  let scanStageLabel = $derived(
    SCAN_STAGES.find((stage) => stage.key === scanStage)?.label ?? '检测完成',
  );
  let displayStageLabel = $derived(
    isScanning
      ? scanStage === 'analysis' && dnsPending && report
        ? '正在检查 WebRTC、DNS、IPv6'
        : scanStageLabel
      : dnsPending
        ? '正在检查 WebRTC、DNS、IPv6'
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
    const next = SCAN_STAGES.find((item) => !completedStages.includes(item.key));
    if (next) scanStage = next.key;
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

  // ============ 扫描管线 ============

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
    const devicePromise = collectDeviceSignals();
    const fpjsPromise = collectFingerprintAudit();
    const environmentReady = Promise.all([fingerprintPromise, devicePromise, fpjsPromise]).then(() => {
      onStageComplete?.('environment');
    });

    const [fingerprint, environment, leaks, device, fpjs] = await Promise.all([
      fingerprintPromise,
      environmentPromise,
      leaksPromise,
      devicePromise,
      fpjsPromise,
    ]);
    await environmentReady;
    // RTT 刻意放在上面这批采集全部结算之后再串行探测：canvas/webgl/audio 是主线程
    // CPU 密集型任务，与探测并发会通过 CPU 争用推迟 fetch 回调与 performance.now() 读数，
    // 只会把 RTT 读偏大（噪声单向为正）。min 是承重统计量，测量质量高于这点串行延迟
    // （典型仅新增 6 次往返的耗时，且有 ~3s 预算封顶，绝不阻塞扫描）。
    const rtt = await collectRTT();
    return { collectors: { fingerprint, environment, leaks, network: { rtt } }, device, fpjs };
  }

  function buildRequest(s: ScanSignals): ScanRequest {
    return {
      scan_id: crypto?.randomUUID ? crypto.randomUUID() : `scan_${Date.now()}`,
      timestamp: Date.now(),
      scenario,
      fingerprint: s.collectors.fingerprint as Record<string, unknown>,
      environment: s.collectors.environment as Record<string, unknown>,
      leaks: s.collectors.leaks as Record<string, unknown>,
      fpjs_audit: s.fpjs,
      network: { rtt: s.collectors.network?.rtt },
    };
  }

  /** ApiError.message 含请求 URL 与响应体原文，不能直接呈现给用户。 */
  function friendlyError(err: unknown): string {
    if (err instanceof api.ApiError) {
      // api.ts 以 status 0 表示超时/网络失败，而非后端应答
      if (err.status === 0) return '网络异常，请检查连接后重试';
      if (err.status === 429) return '请求过于频繁，请稍后再试';
      if (err.status >= 500) return '服务暂时不可用，请稍后重试';
      return '检测请求被拒绝，请刷新页面重试';
    }
    return '检测失败，请稍后重试';
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
      const resp = await api.createScan(buildRequest(signals));
      const synced =
        resp.scenario && resp.scenario !== scenario
          ? await api.getScan(resp.scan_id, scenario).catch(() => resp)
          : resp;
      report = buildReport(synced, signals.collectors, signals.device);
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
      // 新后端已用权威观测重算，旧后端仍走原来的更新接口。
      let updated = dns.data.assessed
        ? await api.getScan(scanId, targetScenario)
        : await api.updateScanDNS(scanId, dns.data.leaked);
      if (scenario !== targetScenario && dns.data.assessed) {
        updated = await api.getScan(scanId, scenario);
      }
      if (report?.meta.scanId === scanId && lastSignals) {
        report = buildReport(updated, lastSignals.collectors, lastSignals.device);
      }
    } catch {
      /* 忽略：DNS 富化失败不影响已呈现的报告 */
    }
  }

  async function selectScenario(next: ScanScenario) {
    if (scenario === next) return;
    scenario = next;
    try {
      window.localStorage.setItem(SCENARIO_KEY, next);
    } catch {
      /* 忽略无痕/禁用存储 */
    }
    trackEvent('scenario-switch', { to: next });

    const currentId = report?.meta.scanId;
    const signals = lastSignals;
    if (!currentId || !signals) return;

    const token = ++scenarioSwitchToken;
    try {
      const updated = await api.getScan(currentId, next);
      if (token !== scenarioSwitchToken || report?.meta.scanId !== currentId) return;
      report = buildReport(updated, signals.collectors, signals.device);
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
      showToast(`优惠码 ${code} 已复制`);
    } catch {
      showToast('复制失败，请检查浏览器权限');
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
      /* 忽略无痕/禁用存储 */
    }
    if (isAutomatedCrawler()) {
      isScanning = false;
      dnsPending = false;
      scanStage = 'complete';
      return;
    }
    runScan('auto');
    // bfcache：浏览器后退会整页恢复旧状态，onMount 不再触发，需手动重扫
    const onPageShow = (e: PageTransitionEvent) => {
      if (e.persisted) runScan('auto');
    };
    window.addEventListener('pageshow', onPageShow);
    return () => {
      revealTimers.forEach((timer) => window.clearTimeout(timer));
      window.removeEventListener('pageshow', onPageShow);
    };
  });

  // ============ 截图 / 分享 / Toast ============

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
      // 分享卡由 Canvas 从报告数据直接生成（IP 脱敏），与页面布局和用户环境解耦
      const blob = await renderShareCard(report, {
        riskLabel: RISK_LABEL[report.riskLevel] ?? report.riskLevel,
      });
      captureProgress = 100;
      let clipboardOk = false;
      try {
        await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
        clipboardOk = true;
      } catch {
        /* 剪贴板权限未开 */
      }
      downloadScreenshot(blob);
      trackEvent('share-save-image', { page: 'home' });
      showToast(clipboardOk ? '分享图已复制并弹出下载' : '已弹出下载窗口（剪贴板权限未开）');
    } catch (error) {
      showToast('分享图生成失败：' + (error as Error).message);
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
    showToast(await copyShareText(formatHomeShareText(report)));
  }
</script>

<AppHeader
  onCopyText={handleCopyText}
  onScreenshot={handleScreenshot}
  onRescan={() => runScan('retest')}
  isScanning={isScanning}
  ready={!!report && !isScanning}
/>

<main class="report-main flex-1 pt-6 sm:pt-10">
  <!-- 标题与模式切换 -->
  <div class="mb-5 sm:mb-8 flex flex-col gap-3.5 sm:flex-row sm:items-end sm:justify-between min-w-0">
    <div class="min-w-0">
      <h1 class="report-title text-2xl sm:text-3xl md:text-4xl mb-1.5 sm:mb-2">
        网络环境一致性检测<span class="text-(--radar-400)">雷达</span>
      </h1>
      <p class="text-sm text-(--slate-400) leading-relaxed">{SCENARIO_META[scenario].subtitle}</p>
    </div>

    <div
      class="grid grid-cols-2 w-full sm:inline-flex sm:w-auto items-center gap-1 rounded-lg border border-white/10 bg-slate-900/80 p-1 shrink-0"
      role="radiogroup"
      aria-label="检测模式"
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
        >{SCENARIO_META[mode].label}</button>
      {/each}
    </div>
  </div>

    <div data-nosnippet>
    <!-- 评分 + 判定 -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 mb-6">
      <div class="lg:col-span-4">
        <RadarCard
          groups={report?.groups ?? (PLACEHOLDER_GROUPS as any)}
          verdictState={report?.verdict.state ?? 'unknown'}
          riskLabel={report ? (RISK_LABEL[report.riskLevel] ?? report.riskLevel) : '—'}
          isScanning={isScanning}
          pendingKeys={dnsPending ? ['leak'] : []}
          settledKeys={revealedKeys}
          activeKey={activeDimension}
          scanStageLabel={displayStageLabel}
        />
      </div>
      <div class="lg:col-span-8">
        <DiagnosisCard report={report} isScanning={isScanning} active={activeDimension === 'identity'} onCopyPromo={handleCopyPromo} onStart={() => runScan('manual')} />
      </div>
    </div>

    {#if scanError}
      <div class="glass-card rounded-xl p-8 text-center">
        <i class="fa-solid fa-plug-circle-xmark text-2xl mb-3" style="color:var(--leak)"></i>
        <p class="text-[15px] text-white">检测未完成</p>
        <p class="mt-1.5 text-xs text-(--slate-500) max-w-md mx-auto">{scanError}</p>
        <button
          onclick={() => runScan('retest')}
          class="mt-5 text-sm px-4 py-2 rounded-lg transition-colors cursor-pointer"
          style="color:var(--radar-400); background:var(--sig-soft)"
        >重新检测</button>
      </div>
    {:else}
      <!-- 网络身份 -->
      <div class="mb-6">
        <IdentityCard
          identity={report?.identity ?? EMPTY_IDENTITY}
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

      <!-- 三项明细 -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <ReadoutCard
          title="环境一致性" icon="fa-globe"
          rows={report?.consistency ?? []}
          state={report?.groups[1]?.state ?? 'unknown'}
          note={report?.groups[1]?.note ?? ''}
          referenceOnly={scenario === 'daily'}
          isScanning={isScanning} settled={revealedKeys.includes('consistency')} active={activeDimension === 'consistency'}
        />
        <ReadoutCard
          title="DNS/IP 泄露" icon="fa-shield-halved"
          rows={report?.leaks ?? []}
          state={report?.groups[2]?.state ?? 'unknown'}
          note={report?.groups[2]?.note ?? ''}
          isScanning={isScanning} settled={revealedKeys.includes('leak')} active={activeDimension === 'leak'}
        />
        <ReadoutCard
          title="环境指纹" icon="fa-fingerprint"
          rows={report?.environment ?? []}
          state={report?.groups[3]?.state ?? 'unknown'}
          note={report?.groups[3]?.note ?? ''}
          isScanning={isScanning} settled={revealedKeys.includes('environment')} active={activeDimension === 'environment'}
        />
      </div>

      <!-- 结果反馈：报告落地后才出现，低调不与主判定争视觉 -->
      {#if report && !isScanning}
        <div class="mt-8">
          <FeedbackCard scanId={report.meta.scanId} />
        </div>
      {/if}
    {/if}
    </div>
  </main>

  <ReportToast bind:this={toast} suppressed={isCapturing} />

  <!-- 截图进度 -->
  {#if isCapturing}
    <div class="fixed inset-0 z-50 pointer-events-none">
      <div class="absolute bottom-8 left-1/2 -translate-x-1/2 w-[320px]">
        <div class="glass-card rounded-lg px-4 py-3">
          <div class="text-xs text-(--slate-300) font-mono">生成截图 · {Math.round(captureProgress)}%</div>
          <div class="mt-2 h-1.5 bg-slate-800 overflow-hidden rounded">
            <div class="h-full transition-all duration-200" style="width:{captureProgress}%; background:var(--radar-400)"></div>
          </div>
        </div>
      </div>
    </div>
  {/if}
