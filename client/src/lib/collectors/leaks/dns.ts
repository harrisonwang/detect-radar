/**
 * DNS 泄露采集器
 *
 * 协议（对齐后端自建权威 NS + DNSTap 记录）：
 *   1. POST /leaks/dns 领取一次性测试域名 {testID}.{zone}
 *   2. 客户端解析 r{rand}-{testID}.{zone}（触发本机 resolver 向权威 NS 查询）
 *   3. 采样结束后 GET /leaks/dns/:id，后端回放它实际观测到的 resolver 出口
 *
 * 依赖后端 DNS 基础设施（权威 NS 委派 + DNSTap）。本地开发无委派时，
 * 观测不到查询 → 返回空 resolver 列表（leaked=false），静默降级，绝不阻塞扫描。
 */

import type { CollectorResult, DNSLeakData } from '../types';
import { createDNSLeakTest, getDNSLeakResult } from '../../api';

/** 触发对测试域名的 DNS 解析（HTTP 是否成功无所谓，解析动作已发生） */
function triggerResolution(testDomain: string): void {
  // 随机标签避免 resolver/浏览器缓存命中。用 `-` 并入 testID 所在 label 保持单级子域：
  // 多一级子域会超出 *.{zone} 单层通配证书的覆盖范围，pixel.gif 必然 TLS 失败刷控制台报错
  const label = `r${Math.floor(performance.now())}${Math.floor(performance.now() * 7) % 9973}`;
  const url = `https://${label}-${testDomain}/pixel.gif?t=${Date.now()}`;
  try {
    const img = new Image();
    img.referrerPolicy = 'no-referrer';
    img.src = url;
  } catch {
    /* noop：解析请求已发出 */
  }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// 给稍晚到达的运营商 DNS 留出时间，避免把最先返回的公共 DNS 当成完整结果。
const OBSERVATION_WINDOW_MS = 6200;

async function startDNSProbe(scanId?: string) {
  const test = await createDNSLeakTest(scanId);
  triggerResolution(test.test_domain);
  await sleep(400);
  triggerResolution(test.test_domain);
  return test.id;
}

async function observeDNS(scanId?: string) {
  const id = await startDNSProbe(scanId);
  await sleep(OBSERVATION_WINDOW_MS);
  return getDNSLeakResult(id);
}

export async function sampleDNSRound() {
  return (await observeDNS()).dns_servers;
}

/**
 * 采集 DNS 泄露。best-effort：整体预算约 7s，失败即降级为 unsupported。
 * scanId 透传给服务端，遥测流水据此把异步结果关联回所属扫描。
 */
export async function collectDNSLeak(scanId?: string): Promise<CollectorResult<DNSLeakData>> {
  const start = performance.now();
  try {
    const result = await observeDNS(scanId);
    return {
      status: 'success',
      data: {
        leaked: result.leaked,
        assessed: !!result.assessment,
        test_id: result.id,
        resolvers: result.dns_servers.map((server) => server.ip).filter(Boolean),
      },
      timing: performance.now() - start,
    };
  } catch (err) {
    // 后端无 DNS 委派 / 端点不可用：降级，不影响主扫描
    return {
      status: 'unsupported',
      error: err instanceof Error ? err.message : 'dns leak test unavailable',
      timing: performance.now() - start,
    };
  }
}
