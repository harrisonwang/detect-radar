
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
