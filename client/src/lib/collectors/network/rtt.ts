
import type { CollectorResult, RTTData } from '../types';
import { API_BASE } from '../../api';

const TOTAL_PROBES = 7;
const WARMUP_DISCARD = 1;
const MIN_USABLE = 3;
const BUDGET_MS = 3000;

const round1 = (x: number) => Math.round(x * 10) / 10;

function createProbeID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    const bytes = new Uint8Array(8);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  }
  return (
    Date.now().toString(36) +
    Math.random().toString(36).slice(2) +
    Math.random().toString(36).slice(2)
  )
    .replace(/[^a-z0-9]/gi, '')
    .slice(0, 16)
    .padEnd(8, '0');
}

function median(sorted: number[]): number {
  const n = sorted.length;
  const mid = Math.floor(n / 2);
  return n % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

function readConnectTiming(urls: string[]): Pick<RTTData, 'connect_ms' | 'conn_reused'> {
  try {
    if (typeof performance.getEntriesByType !== 'function' || urls.length === 0) return {};
    const wanted = new Set(urls);
    const entries = performance.getEntriesByType('resource') as PerformanceResourceTiming[];
    let matched = false;
    let maxConnect = 0;
    for (const e of entries) {
      if (!wanted.has(e.name)) continue;
      matched = true;
      const c = e.connectEnd - e.connectStart;
      if (c > maxConnect) maxConnect = c;
    }
    if (!matched) return {};
    if (maxConnect > 0) return { connect_ms: round1(maxConnect), conn_reused: false };
    return { conn_reused: true };
  } catch {
    return {};
  }
}

export async function collectRTT(): Promise<CollectorResult<RTTData>> {
  if (
    typeof fetch !== 'function' ||
    typeof performance === 'undefined' ||
    typeof performance.now !== 'function'
  ) {
    return { status: 'unsupported', error: 'fetch/performance 不可用' };
  }

  const startWall = performance.now();
  const probeId = createProbeID();
  const samples: number[] = [];
  const probeUrls: string[] = [];

  try {
    for (let i = 0; i < TOTAL_PROBES; i++) {
      const remaining = BUDGET_MS - (performance.now() - startWall);
      if (remaining <= 0) break;

      const url = `${API_BASE}/ping?probe_id=${probeId}&_=${Date.now()}-${i}`;
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), remaining);
      const t0 = performance.now();
      try {
        await fetch(url, { method: 'GET', cache: 'no-store', signal: controller.signal });
        const dt = performance.now() - t0;
        probeUrls.push(url);
        if (i >= WARMUP_DISCARD) samples.push(round1(dt));
      } catch {
      } finally {
        clearTimeout(timer);
      }
    }

    if (samples.length < MIN_USABLE) {
      return {
        status: 'error',
        error: `有效 RTT 样本不足 (${samples.length}/${MIN_USABLE})`,
        timing: round1(performance.now() - startWall),
      };
    }

    const sorted = [...samples].sort((a, b) => a - b);
    const min_ms = sorted[0];
    const max_ms = sorted[sorted.length - 1];

    return {
      status: 'success',
      data: {
        probe_id: probeId,
        samples,
        min_ms,
        median_ms: round1(median(sorted)),
        jitter_ms: round1(max_ms - min_ms),
        count: samples.length,
        ...readConnectTiming(probeUrls),
      },
      timing: round1(performance.now() - startWall),
    };
  } catch (e) {
    return {
      status: 'error',
      error: e instanceof Error ? e.message : 'rtt probe failed',
      timing: round1(performance.now() - startWall),
    };
  }
}
