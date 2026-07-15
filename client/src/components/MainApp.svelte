<script lang="ts">
  import { onMount } from 'svelte';

  import AppHeader from './report/AppHeader.svelte';
  import RadarCard from './report/RadarCard.svelte';
  import DiagnosisCard from './report/DiagnosisCard.svelte';
  import IdentityCard from './report/IdentityCard.svelte';
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
  import { buildReport, type ReportModel, type IdentityReadout } from '../lib/report';
  import { renderShareCard } from '../lib/shareCard';
  import type { ScanRequest } from '../lib/types';

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

  // 检测中的占位四轴（雷达在扫描态仍显示四轴骨架）
  const PLACEHOLDER_GROUPS = [
    { key: 'identity', label: '网络身份', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
    { key: 'consistency', label: '环境一致性', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
    { key: 'leak', label: 'DNS/IP 泄露', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
    { key: 'environment', label: '环境指纹', score: 0, state: 'unknown', passed: 0, total: 1, note: '' },
  ] as const;

  let report = $state<ReportModel | null>(null);
  let isScanning = $state(true);
  let scanError = $state<string | null>(null);
  // DNS 泄露走独立慢路径（enrichDNS 约 4s），未定前雷达该轴显示待定而非中间值
  let dnsPending = $state(true);
  let ipRevealed = $state(true);
  let lastSignals: ScanSignals | null = null;

  // ============ 扫描管线 ============

  async function collectSignals(): Promise<ScanSignals> {
    const [fingerprint, environment, leaks, device, fpjs] = await Promise.all([
      collectFingerprints(),
      collectEnvironment(),
      collectLeaks(),
      collectDeviceSignals(),
      collectFingerprintAudit(),
    ]);
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
      scenario: 'general',
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

  async function runScan() {
    isScanning = true;
    scanError = null;
    dnsPending = true;
    try {
      const signals = await collectSignals();
      lastSignals = signals;
      const resp = await api.createScan(buildRequest(signals));
      report = buildReport(resp, signals.collectors, signals.device);
      void enrichDNS().finally(() => (dnsPending = false));
    } catch (err) {
      const msg = friendlyError(err);
      scanError = msg;
      report = null;
      dnsPending = false;
      showToast(msg);
    } finally {
      isScanning = false;
    }
  }

  async function enrichDNS() {
    try {
      const scanId = report?.meta.scanId;
      const dns = await collectDNSLeak(scanId);
      if (!report || !scanId || dns.status !== 'success' || !dns.data || dns.data.resolvers.length === 0) return;
      // 纯后端评分：把 DNS 结果回传后端重算，再用返回的响应整体重建报告
      const updated = await api.updateScanDNS(scanId, dns.data.leaked);
      if (lastSignals) report = buildReport(updated, lastSignals.collectors, lastSignals.device);
    } catch {
      /* 忽略：DNS 富化失败不影响已呈现的报告 */
    }
  }

  async function handleCopyIP() {
    const ip = report?.identity.ip;
    if (!ip) return;
    try {
      await navigator.clipboard.writeText(ip);
      showToast('IP 已复制');
    } catch {
      showToast('复制失败，请检查浏览器权限');
    }
  }

  function toggleReveal() {
    ipRevealed = !ipRevealed;
  }

  onMount(() => {
    runScan();
    // bfcache：浏览器后退会整页恢复旧状态，onMount 不再触发，需手动重扫
    const onPageShow = (e: PageTransitionEvent) => {
      if (e.persisted) runScan();
    };
    window.addEventListener('pageshow', onPageShow);
    return () => window.removeEventListener('pageshow', onPageShow);
  });

  // ============ 截图 / 分享 / Toast ============

  let toastVisible = $state(false);
  let toastMessage = $state('');
  let isCapturing = $state(false);
  let captureProgress = $state(0);
  let captureTimer: number | null = null;

  function showToast(message: string) {
    toastMessage = message;
    toastVisible = true;
    setTimeout(() => (toastVisible = false), 3000);
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
      toastVisible = false;
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

  async function handleShare() {
    try {
      await navigator.clipboard.writeText(window.location.href);
      showToast('页面链接已复制');
    } catch {
      showToast('复制失败，请检查浏览器权限');
    }
  }
</script>

<div class="min-h-screen bg-space-950 scan-grid relative">
  <!-- 背景光晕 -->
  <div class="fixed inset-0 overflow-hidden pointer-events-none">
    <div class="absolute top-[-10%] left-[-10%] w-125 h-125 bg-(--radar-500)/10 rounded-full blur-[120px]"></div>
    <div class="absolute bottom-[-10%] right-[-10%] w-150 h-150 bg-blue-600/10 rounded-full blur-[120px]"></div>
  </div>

  <AppHeader
    onScreenshot={handleScreenshot}
    onShare={handleShare}
    isScanning={isScanning}
    ready={!!report && !isScanning}
  />

  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10 relative z-10">
    <!-- 标题 -->
    <div class="mb-8">
      <h1 class="text-3xl md:text-4xl font-bold text-white mb-2">
        网络环境一致性检测<span class="text-(--radar-400)">雷达</span>
      </h1>
    </div>

    <!-- 评分 + 判定 -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 mb-6">
      <div class="lg:col-span-4">
        <RadarCard
          groups={report?.groups ?? (PLACEHOLDER_GROUPS as any)}
          verdictState={report?.verdict.state ?? 'unknown'}
          riskLabel={report ? (RISK_LABEL[report.riskLevel] ?? report.riskLevel) : '—'}
          isScanning={isScanning}
          pendingKeys={dnsPending ? ['leak'] : []}
        />
      </div>
      <div class="lg:col-span-8">
        <DiagnosisCard report={report} isScanning={isScanning} />
      </div>
    </div>

    {#if scanError}
      <div class="glass-card rounded-xl p-8 text-center reveal">
        <i class="fa-solid fa-plug-circle-xmark text-2xl mb-3" style="color:var(--leak)"></i>
        <p class="text-[15px] text-white">检测未完成</p>
        <p class="mt-1.5 text-xs text-(--slate-500) max-w-md mx-auto">{scanError}</p>
        <button
          onclick={runScan}
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
          revealed={ipRevealed}
          onToggleReveal={toggleReveal}
          onCopy={handleCopyIP}
        />
      </div>

      <!-- 三项明细 -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <ReadoutCard
          title="环境一致性" icon="fa-globe"
          rows={report?.consistency ?? []}
          state={report?.groups[1]?.state ?? 'unknown'}
          note={report?.groups[1]?.note ?? ''}
          isScanning={isScanning} delay={40}
        />
        <ReadoutCard
          title="DNS/IP 泄露" icon="fa-shield-halved"
          rows={report?.leaks ?? []}
          state={report?.groups[2]?.state ?? 'unknown'}
          note={report?.groups[2]?.note ?? ''}
          isScanning={isScanning} delay={100}
        />
        <ReadoutCard
          title="环境指纹" icon="fa-fingerprint"
          rows={report?.environment ?? []}
          state={report?.groups[3]?.state ?? 'unknown'}
          note={report?.groups[3]?.note ?? ''}
          isScanning={isScanning} delay={160}
        />
      </div>

      <!-- 结果反馈：报告落地后才出现，低调不与主判定争视觉 -->
      {#if report && !isScanning}
        <div class="mt-8">
          <FeedbackCard scanId={report.meta.scanId} />
        </div>
      {/if}
    {/if}
  </main>

  <footer class="border-t border-slate-800 mt-12 py-8 relative z-10">
    <div class="max-w-7xl mx-auto px-4 flex flex-col md:flex-row justify-between items-center text-(--slate-500) text-sm gap-3">
      <p>© 2026 DetectRadar.com</p>
      <div class="flex gap-4">
        <a href="/privacy" class="hover:text-white transition">隐私政策</a>
        <a href="/terms" class="hover:text-white transition">服务条款</a>
      </div>
    </div>
  </footer>

  <!-- Toast -->
  {#if toastVisible && !isCapturing}
    <div class="fixed bottom-6 right-6 z-50 reveal">
      <div class="glass-card rounded-lg flex items-center gap-2.5 px-4 py-3 shadow-xl">
        <i class="fa-solid fa-circle-check" style="color:var(--sig)"></i>
        <span class="text-sm text-white">{toastMessage}</span>
      </div>
    </div>
  {/if}

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
</div>
