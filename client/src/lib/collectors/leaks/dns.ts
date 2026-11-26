
import type { Locale } from '../../../i18n';
import type { CollectorResult, DNSLeakData } from '../types';
import { createDNSLeakTest, getDNSLeakResult } from '../../api';

function triggerResolution(testDomain: string): void {
  const label = `r${Math.floor(performance.now())}${Math.floor(performance.now() * 7) % 9973}`;
  const url = `https://${label}-${testDomain}/pixel.gif?t=${Date.now()}`;
  try {
    const img = new Image();
    img.referrerPolicy = 'no-referrer';
    img.src = url;
  } catch {
  }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

const OBSERVATION_WINDOW_MS = 6200;

async function startDNSProbe(scanId?: string, locale?: Locale) {
  const test = await createDNSLeakTest(scanId, locale);
  triggerResolution(test.test_domain);
  await sleep(400);
  triggerResolution(test.test_domain);
  return test.id;
}

async function observeDNS(scanId?: string, locale?: Locale) {
  const id = await startDNSProbe(scanId, locale);
  await sleep(OBSERVATION_WINDOW_MS);
  return getDNSLeakResult(id, locale);
}

export async function sampleDNSRound(locale?: Locale) {
  return (await observeDNS(undefined, locale)).dns_servers;
}

export async function collectDNSLeak(scanId?: string, locale?: Locale): Promise<CollectorResult<DNSLeakData>> {
  const start = performance.now();
  try {
    const result = await observeDNS(scanId, locale);
    return {
      status: 'success',
      data: {
        test_id: result.id,
        resolvers: result.dns_servers.map((server) => server.ip).filter(Boolean),
      },
      timing: performance.now() - start,
    };
  } catch (err) {
    return {
      status: 'unsupported',
      error: err instanceof Error ? err.message : 'dns leak test unavailable',
      timing: performance.now() - start,
    };
  }
}
