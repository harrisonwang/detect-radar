import { ApiError, assessDNSObservation, getBrowserExit } from './api';
import { sampleDNSRound } from './collectors/leaks/dns';
import { DNS_POLICY_VERSION, browserExit, dnsExitType, dnsProvider, dnsProviderName, dnsRegion } from './dnsPresentation';
import type { DNSExit, DNSResolver, DNSVerdict } from './types';

const DNS_ROUNDS = 3;

export interface DNSCheck {
  time: number;
  exit: DNSExit | null;
  resolvers: DNSResolver[];
  observedRounds: number;
  totalRounds: number;
  // Results saved by the first version have samples but no stored verdict.
  verdict?: DNSVerdict;
}

type FailureKind = 'empty' | 'limited' | 'unavailable';

export class DNSCheckError extends Error {
  constructor(readonly kind: FailureKind) {
    super(kind);
  }
}

export async function withDNSAssessment(check: DNSCheck): Promise<DNSCheck> {
  if (check.verdict?.policy_version === DNS_POLICY_VERSION) return check;
  try {
    const result = await assessDNSObservation({
      exit: check.exit, resolvers: check.resolvers,
      observed_rounds: check.observedRounds, total_rounds: check.totalRounds,
    });
    if (result.verdict.policy_version === DNS_POLICY_VERSION) return { ...check, ...result };
  } catch { /* Keep the samples so a later visit can retry the assessment. */ }
  return { ...check, verdict: { state: 'unknown', reason: 'assessment-unavailable' } };
}

export async function measureDNS(onExit: (exit: DNSExit) => void): Promise<DNSCheck> {
  let exit: DNSExit | null = null;
  const exitRequest = getBrowserExit().then((info) => {
    exit = browserExit(info);
    onExit(exit);
  }).catch(() => { /* DNS samples are useful even if the exit request fails. */ });

  // Each round uses a separate probe name. Staggering samples covers resolver rotation.
  const rounds = Array.from({ length: DNS_ROUNDS }, async (_, index) => {
    if (index) await new Promise<void>((resolve) => setTimeout(resolve, index * 1000));
    return sampleDNSRound();
  });
  const [results] = await Promise.all([Promise.allSettled(rounds), exitRequest]);
  const resolvers = new Map<string, DNSResolver>();
  let observedRounds = 0;

  for (const round of results) {
    if (round.status !== 'fulfilled') continue;
    const seen = new Set<string>();
    for (const { ip, country, isp, asn, network_type } of round.value) {
      if (!ip || seen.has(ip)) continue;
      seen.add(ip);
      const stored = resolvers.get(ip);
      resolvers.set(ip, {
        ip, country: (country || stored?.country || '').trim().toUpperCase(),
        isp: dnsProvider(isp) || stored?.isp || '',
        asn: asn || stored?.asn, network_type: network_type ?? stored?.network_type,
        rounds: (stored?.rounds ?? 0) + 1,
      });
    }
    if (seen.size) observedRounds++;
  }

  if (!observedRounds) {
    const errors = results.filter((round) => round.status === 'rejected');
    if (errors.some((round) => round.reason instanceof ApiError && round.reason.status === 429)) {
      throw new DNSCheckError('limited');
    }
    throw new DNSCheckError(errors.length === DNS_ROUNDS ? 'unavailable' : 'empty');
  }

  return withDNSAssessment({
    time: Date.now(), exit, resolvers: [...resolvers.values()], observedRounds, totalRounds: DNS_ROUNDS,
  });
}

function suspectIPs(check: DNSCheck): Set<string> {
  return new Set(check.verdict?.suspect_ips);
}

function sortedResolvers(check: DNSCheck) {
  const suspects = suspectIPs(check);
  return [...check.resolvers].sort((a, b) =>
    Number(suspects.has(b.ip)) - Number(suspects.has(a.ip)) ||
    b.rounds - a.rounds || a.ip.localeCompare(b.ip));
}

interface DNSNetwork {
  providerKey: string;
  name: string;
  country: string;
  type?: DNSResolver['network_type'];
  suspect: boolean;
  samples: DNSResolver[];
}

export function dnsNetworks(check: DNSCheck): Map<string, DNSNetwork> {
  const groups = new Map<string, DNSNetwork>();
  const suspects = suspectIPs(check);
  for (const resolver of sortedResolvers(check)) {
    const name = dnsProviderName(resolver.isp);
    const { country, network_type: type } = resolver;
    const providerKey = name ? `provider:${name.toLowerCase()}` : resolver.asn || resolver.ip;
    const key = `${providerKey}|${country}`;
    const existing = groups.get(key);
    if (existing) {
      existing.samples.push(resolver);
    } else {
      groups.set(key, { providerKey, name, country, type, suspect: suspects.has(resolver.ip), samples: [resolver] });
    }
  }
  return groups;
}

export function dnsProviderGroups(networks: Iterable<DNSNetwork>) {
  const providers = new Map<string, DNSNetwork[]>();
  for (const network of networks) {
    const regions = providers.get(network.providerKey);
    if (regions) regions.push(network);
    else providers.set(network.providerKey, [network]);
  }
  return [...providers].map(([key, regions]) => ({
    key,
    name: regions[0].name || '网络未知',
    regions,
    suspect: regions.some((region) => region.suspect),
    type: regions.every((region) => region.type === regions[0].type) ? regions[0].type : undefined,
  }));
}

function networkLabels(networks: DNSNetwork[], other: Set<string>) {
  return dnsProviderGroups(networks).map(({ key, name, regions }) =>
    other.has(key) ? `${name}（${regions.map((region) => dnsRegion(region.country)).join('、')}）` : name
  ).join('、');
}

function exitLabel(exit: DNSExit) {
  const region = dnsRegion(exit.country);
  const provider = dnsProviderName(exit.network) || exit.asn;
  if (!provider) return region;
  return provider.startsWith(region) ? provider : `${region} · ${provider}`;
}

export function compareDNS(previous: DNSCheck, current: DNSCheck, after: Map<string, DNSNetwork>) {
  const before = dnsNetworks(previous);
  const beforeProviders = new Set([...before.values()].map((network) => network.providerKey));
  const afterProviders = new Set([...after.values()].map((network) => network.providerKey));
  const removed = [...before].filter(([key]) => !after.has(key)).map(([, n]) => n);
  const added = [...after].filter(([key]) => !before.has(key)).map(([, n]) => n);
  const oldDNS = networkLabels(removed, afterProviders);
  const newDNS = networkLabels(added, beforeProviders);
  const beforeIPs = new Set(previous.resolvers.map((r) => r.ip));
  const afterIPs = new Set(current.resolvers.map((r) => r.ip));
  const addedIPs = new Set([...afterIPs].filter((ip) => !beforeIPs.has(ip)));
  const removedIPs = [...beforeIPs].filter((ip) => !afterIPs.has(ip));
  const dnsIPsChanged = addedIPs.size > 0 || removedIPs.length > 0;
  const sameDNS = !added.length && !removed.length && [...before].every(([key, n]) =>
    after.get(key)?.type === n.type && after.get(key)?.suspect === n.suspect);
  const dnsChanges: string[] = [];
  if (oldDNS && newDNS && removed.length === before.size && added.length === after.size) {
    dnsChanges.push(`${oldDNS} → ${newDNS}`);
  } else {
    if (oldDNS) dnsChanges.push(`本次未再测到${/^[A-Za-z0-9]/.test(oldDNS) ? ' ' : ''}${oldDNS}`);
    if (newDNS) dnsChanges.push(`本次新增${/^[A-Za-z0-9]/.test(newDNS) ? ' ' : ''}${newDNS}`);
  }
  if (!dnsChanges.length && !sameDNS) dnsChanges.push('网络归属信息有变化');
  if (!dnsChanges.length && dnsIPsChanged) dnsChanges.push('仅服务器地址有变化');

  const oldExit = previous.exit;
  const newExit = current.exit;
  let exitChange = '';
  if (!oldExit?.ip || !newExit?.ip) {
    exitChange = !oldExit?.ip && !newExit?.ip ? '两次均未获取' : !oldExit?.ip ? '上次未获取' : '本次未获取';
  } else if (exitLabel(oldExit) !== exitLabel(newExit)) {
    exitChange = `${exitLabel(oldExit)} → ${exitLabel(newExit)}`;
  } else if (oldExit.ip !== newExit.ip) {
    exitChange = '出口 IP 有变化';
  } else if (oldExit.asn !== newExit.asn || dnsExitType(oldExit) !== dnsExitType(newExit)) {
    exitChange = '网络归属信息有变化';
  }

  const beforeStatus = previous.verdict?.state;
  const afterStatus = current.verdict?.state;
  const previousVerdict = beforeStatus !== afterStatus ? beforeStatus : undefined;
  const stable = !exitChange && sameDNS && !previousVerdict && previous.verdict?.reason === current.verdict?.reason
    && afterStatus !== 'unknown' && previous.observedRounds === previous.totalRounds
    && current.observedRounds === current.totalRounds;
  return {
    previousVerdict,
    improved: beforeStatus === 'suspected' && afterStatus === 'clear',
    summary: stable ? (dnsIPsChanged ? 'DNS 服务商未变，地址有变化。' : '与上次一致') : '',
    dnsChanges, exitChange, addedIPs, removedIPs,
  };
}
