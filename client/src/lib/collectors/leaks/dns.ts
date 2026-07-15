/**
 * DNS 泄露采集器
 *
 * 协议（对齐后端自建权威 NS + DNSTap 记录）：
 *   1. POST /leaks/dns 领取一次性测试域名 {testID}.{zone}
 *   2. 客户端解析 r{rand}-{testID}.{zone}（触发本机 resolver 向权威 NS 查询）
 *   3. 轮询 GET /leaks/dns/:id，后端回放它实际观测到的 resolver 出口
 *
 * 依赖后端 DNS 基础设施（权威 NS 委派 + DNSTap）。本地开发无委派时，
 * 观测不到查询 → 返回空 resolver 列表（leaked=false），静默降级，绝不阻塞扫描。
 */

import type { CollectorResult, DNSLeakData } from '../types';
import { createDNSLeakTest, getDNSLeakResult, ApiError } from '../../api';

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

/**
 * 轮询退避（毫秒），累计休眠 6.2s。
 * 国内递归解析器对随机子域的冷查询实测尾部可达 3.4s（itdog 124 节点全国实测），
 * 旧的 3×1s 固定节奏在 3.4s 处截断，尾部用户的信标观测不到，DNS 轴只能报「未检测」。
 */
const POLL_BACKOFF_MS = [200, 400, 800, 1600, 3200];

/**
 * 采集 DNS 泄露。best-effort：整体预算约 7s，失败即降级为 unsupported。
 * scanId 透传给服务端，遥测流水据此把异步结果关联回所属扫描。
 */
export async function collectDNSLeak(scanId?: string): Promise<CollectorResult<DNSLeakData>> {
  const start = performance.now();
  try {
    const test = await createDNSLeakTest(scanId);

    // 触发两次，提升记录命中率
    triggerResolution(test.test_domain);
    await sleep(400);
    triggerResolution(test.test_domain);

    // 轮询结果（后端异步记录 DNSTap）
    let resolvers: string[] = [];
    let leaked = false;
    for (const wait of POLL_BACKOFF_MS) {
      await sleep(wait);
      try {
        const res = await getDNSLeakResult(test.id);
        resolvers = res.dns_servers.map((q) => q.ip).filter(Boolean);
        leaked = res.leaked;
        if (resolvers.length > 0) break;
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) continue;
        throw err;
      }
    }

    return {
      status: 'success',
      data: { leaked, test_id: test.id, resolvers },
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
