/**
 * IPv6 泄露采集器
 *
 * 并发探测纯 IPv4（4.detectradar.com）与纯 IPv6（6.detectradar.com）双栈出口，
 * 不依赖 WebRTC，也不因用户禁用 WebRTC 而跳过。
 * 当 IPv4 出口在海外、而 IPv6 落在国内宽带运营商网段时，判定存在 IPv6 旁路泄露。
 */

import { measureIPv6 } from '../../ipv6Check';
import type { CollectorResult, IPv6LeakData } from '../types';

export async function collectIPv6Leak(): Promise<CollectorResult<IPv6LeakData>> {
  const t0 = performance.now();
  try {
    const check = await measureIPv6(undefined, { v4Ms: 4000, v6Ms: 2500 });
    return {
      status: 'success',
      data: {
        leaked: check.verdict.state === 'leaked',
        ipv6_address: check.v6?.ip ?? null,
      },
      timing: Math.round(performance.now() - t0),
    };
  } catch (err) {
    return {
      status: 'error',
      error: (err as Error)?.message ?? 'ipv6 probe failed',
      timing: Math.round(performance.now() - t0),
    };
  }
}
