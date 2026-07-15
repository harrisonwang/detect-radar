/**
 * RTT 探测采集器（应用层往返时延）
 *
 * 用「物理时延」反推代理：住宅代理把流量绕经中转节点，应用层 RTT 会显著高于
 * IP 归属地暗示的理论时延。本采集器只负责如实测量并上报，是否判定为异常由后端
 * 在影子模式下离线标定，Phase 1 不参与前端评分、不改变任何报告呈现。
 *
 * 方法学要点：
 *  - 命中后端 GET {API_BASE}/ping（返回 204、无 body）。fetch 在收到响应头即 resolve，
 *    204 无 body → 计时窗口恰好覆盖一次完整的应用层往返。
 *  - performance.now() 在隐私考量下被各浏览器粗化（Safari/Firefox ~1ms 量级）。我们关心
 *    的是「几十毫秒」级的路径差异，1ms 抖动不影响结论，无需更高精度时钟。
 *  - 串行探测，不并行：并行请求相互争抢连接与带宽会系统性推高读数。
 */

import type { CollectorResult, RTTData } from '../types';
import { API_BASE } from '../../api';

/** 总探测次数（含 1 次预热） */
const TOTAL_PROBES = 7;
/** 丢弃前 N 次：首次探测要预热 DNS/TLS/H2 stream 状态，系统性偏慢，不计入统计 */
const WARMUP_DISCARD = 1;
/** 有效样本下限：不足则宁可报 error 也不上报误导性统计 */
const MIN_USABLE = 3;
/** 总预算：整段探测超过 ~3s 立即收手，扫描绝不因 RTT 阻塞 */
const BUDGET_MS = 3000;

/** 时钟被隐私粗化到 ~1ms，保留 0.1ms 精度足够，同时抹掉浮点尾噪 */
const round1 = (x: number) => Math.round(x * 10) / 10;

function median(sorted: number[]): number {
  const n = sorted.length;
  const mid = Math.floor(n / 2);
  return n % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

/**
 * 从 Resource Timing 里机会性读取 TCP+TLS 建连耗时（connectEnd - connectStart）。
 *
 * ⚠️ 关键限制：本站走 HTTP/2 且连接复用——在被复用的连接上
 * connectStart === connectEnd === fetchStart，connect 段为 0（不可得）。
 * 此外若 /ping 跨源且后端未回 Timing-Allow-Origin，该段同样被隐私置零，
 * 二者从 API 层无法区分。因此只在 connect 段 > 0 时才上报 connect_ms，
 * 其余一律视为「连接复用」并置 conn_reused=true，绝不臆造数值。
 */
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
      // 首个探测（新建连接）会带出真实建连耗时，复用的后续探测均为 0，取最大即建连值
      const c = e.connectEnd - e.connectStart;
      if (c > maxConnect) maxConnect = c;
    }
    if (!matched) return {}; // 没找到对应条目：缓冲区被清或不支持，不臆测
    if (maxConnect > 0) return { connect_ms: round1(maxConnect), conn_reused: false };
    return { conn_reused: true }; // connect 段为空 → 连接被复用（或跨源被置零）
  } catch {
    return {};
  }
}

/**
 * 采集应用层 RTT。best-effort：任何失败都返回非 success 的 CollectorResult，
 * 一次失败的探测绝不会中断主扫描。
 */
export async function collectRTT(): Promise<CollectorResult<RTTData>> {
  // 特性检测：缺 fetch/performance 的老环境直接判定不支持
  if (
    typeof fetch !== 'function' ||
    typeof performance === 'undefined' ||
    typeof performance.now !== 'function'
  ) {
    return { status: 'unsupported', error: 'fetch/performance 不可用' };
  }

  const startWall = performance.now();
  const samples: number[] = [];
  const probeUrls: string[] = [];

  try {
    for (let i = 0; i < TOTAL_PROBES; i++) {
      const remaining = BUDGET_MS - (performance.now() - startWall);
      if (remaining <= 0) break; // 预算耗尽，用已有样本收尾

      // 缓存穿透：no-store + 唯一 query 参数确保每次探测都真的走网络。
      // 同源 + H2 下 query 变化不会新建 TCP/TLS 连接（连接按 origin 复用），
      // 仅让请求行不同、绕过缓存，正是我们想要的。
      const url = `${API_BASE}/ping?_=${Date.now()}-${i}`;
      const controller = new AbortController();
      // 单次探测超时挂钩剩余总预算：任一探测都不可能把总耗时拖过 ~3s
      const timer = setTimeout(() => controller.abort(), remaining);
      const t0 = performance.now();
      try {
        // keepalive 无需——探测不跨页面卸载；不设自定义头以免触发 CORS 预检
        await fetch(url, { method: 'GET', cache: 'no-store', signal: controller.signal });
        const dt = performance.now() - t0;
        probeUrls.push(url);
        if (i >= WARMUP_DISCARD) samples.push(round1(dt)); // 丢弃首次预热样本
      } catch {
        // 单次探测失败/超时中断：跳过该样本，继续后续探测
      } finally {
        clearTimeout(timer);
      }
    }

    // 有效样本不足：宁缺毋滥，报 error 而非上报会误导标定的统计
    if (samples.length < MIN_USABLE) {
      return {
        status: 'error',
        error: `有效 RTT 样本不足 (${samples.length}/${MIN_USABLE})`,
        timing: round1(performance.now() - startWall),
      };
    }

    const sorted = [...samples].sort((a, b) => a - b);
    // min 是最承重的统计量：它最贴近真实路径 RTT——排队/GC/CPU 争用等噪声只会「加时」，
    // 永远不会让某次往返比物理下限更快，故取最小值最能逼近纯网络时延。
    const min_ms = sorted[0];
    const max_ms = sorted[sorted.length - 1];

    return {
      status: 'success',
      data: {
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
    // 兜底：任何未预期异常都降级，绝不冒泡到扫描管线
    return {
      status: 'error',
      error: e instanceof Error ? e.message : 'rtt probe failed',
      timing: round1(performance.now() - startWall),
    };
  }
}
