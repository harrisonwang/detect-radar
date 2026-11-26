import { getMessages, joinList, type Locale } from '../i18n';
import { ApiError, assessDNSObservation, getBrowserExit } from './api';
import { sampleDNSRound } from './collectors/leaks/dns';
import { DNS_POLICY_VERSION, browserExit, dnsExitType, dnsProvider, dnsProviderName, dnsRegion } from './dnsPresentation';
import type { DNSExit, DNSResolver, DNSVerdict } from './types';

const DNS_ROUNDS = 3;

export type DNSPhase = 'probing' | 'collecting' | 'assessing';

export interface DNSProgress {
  phase: DNSPhase;
  startedRounds: number;
  completedRounds: number;
  totalRounds: number;
  exit: DNSExit | null;
  resolvers: DNSResolver[];
}

export interface DNSCheck {
  time: number;
  exit: DNSExit | null;
  resolvers: DNSResolver[];
  observedRounds: number;
  totalRounds: number;
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
  } catch {  }
  return { ...check, verdict: { state: 'unknown', reason: 'assessment-unavailable' } };
}

export async function measureDNS(
  onExit: (exit: DNSExit) => void,
  onProgress?: (progress: DNSProgress) => void,
): Promise<DNSCheck> {
  let exit: DNSExit | null = null;
  const resolvers = new Map<string, DNSResolver>();
  let phase: DNSPhase = 'probing';
  let startedRounds = 0;
  let completedRounds = 0;
  let observedRounds = 0;

  const emit = () => {
    onProgress?.({
      phase,
      startedRounds,
      completedRounds,
      totalRounds: DNS_ROUNDS,
      exit,
      resolvers: [...resolvers.values()],
    });
  };

  emit();

  const exitRequest = getBrowserExit().then((info) => {
    exit = browserExit(info);
    onExit(exit);
    emit();
  }).catch(() => {  });

  const rounds = Array.from({ length: DNS_ROUNDS }, async (_, index) => {
    if (index) await new Promise<void>((resolve) => setTimeout(resolve, index * 1000));
    startedRounds++;
    emit();
    const batch = await sampleDNSRound();
    const seen = new Set<string>();
    for (const { ip, country, isp, asn, network_type } of batch) {
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
    completedRounds++;
    if (seen.size) {
      observedRounds++;
      phase = 'collecting';
    }
    emit();
    return batch;
  });

  const [results] = await Promise.all([Promise.allSettled(rounds), exitRequest]);

  if (!observedRounds) {
    const errors = results.filter((round) => round.status === 'rejected');
    if (errors.some((round) => round.reason instanceof ApiError && round.reason.status === 429)) {
      throw new DNSCheckError('limited');
    }
    throw new DNSCheckError(errors.length === DNS_ROUNDS ? 'unavailable' : 'empty');
  }

  phase = 'assessing';
  emit();

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

export function dnsNetworks(check: DNSCheck, locale: Locale = 'zh-CN'): Map<string, DNSNetwork> {
  const groups = new Map<string, DNSNetwork>();
  const suspects = suspectIPs(check);
  for (const resolver of sortedResolvers(check)) {
    const canonicalName = dnsProviderName(resolver.isp, 'en');
    const name = dnsProviderName(resolver.isp, locale);
    const { country, network_type: type } = resolver;
    const providerKey = canonicalName ? `provider:${canonicalName.toLowerCase()}` : resolver.asn || resolver.ip;
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

export function dnsProviderGroups(networks: Iterable<DNSNetwork>, locale: Locale = 'zh-CN') {
  const unknownNetwork = getMessages(locale).common.unknownNetwork;
  const providers = new Map<string, DNSNetwork[]>();
  for (const network of networks) {
    const regions = providers.get(network.providerKey);
    if (regions) regions.push(network);
    else providers.set(network.providerKey, [network]);
  }
  return [...providers].map(([key, regions]) => ({
    key,
    name: regions[0].name || unknownNetwork,
    regions,
    suspect: regions.some((region) => region.suspect),
    type: regions.every((region) => region.type === regions[0].type) ? regions[0].type : undefined,
  }));
}

function networkLabels(networks: DNSNetwork[], other: Set<string>, locale: Locale) {
  return dnsProviderGroups(networks, locale).map(({ key, name, regions }) => {
    if (!other.has(key)) return { name, region: '' };
    const joined = joinList(regions.map((r) => dnsRegion(r.country, locale)), locale);
    return {
      name,
      region: locale === 'en' ? ` (${joined})` : `（${joined}）`,
    };
  });
}

interface DNSChange {
  prefix?: string;
  removed?: ReturnType<typeof networkLabels>;
  text?: string;
}

function exitLabel(exit: DNSExit, locale: Locale) {
  const region = dnsRegion(exit.country, locale);
  const provider = dnsProviderName(exit.network, locale) || exit.asn;
  if (!provider) return region;
  return provider.startsWith(region) ? provider : `${region} · ${provider}`;
}

export function compareDNS(
  previous: DNSCheck,
  current: DNSCheck,
  after: Map<string, DNSNetwork>,
  tense: 'current' | 'previous' = 'current',
  locale: Locale = 'zh-CN',
) {
  const c = getMessages(locale).dns.compare;
  const cur = tense === 'current';
  const before = dnsNetworks(previous, locale);
  const beforeProviders = new Set([...before.values()].map((network) => network.providerKey));
  const afterProviders = new Set([...after.values()].map((network) => network.providerKey));
  const removed = [...before].filter(([key]) => !after.has(key)).map(([, n]) => n);
  const added = [...after].filter(([key]) => !before.has(key)).map(([, n]) => n);
  const oldDNS = networkLabels(removed, afterProviders, locale);
  const newDNS = joinList(
    networkLabels(added, beforeProviders, locale).map(({ name, region }) => name + region),
    locale,
  );
  const beforeIPs = new Set(previous.resolvers.map((r) => r.ip));
  const afterIPs = new Set(current.resolvers.map((r) => r.ip));
  const addedIPs = new Set([...afterIPs].filter((ip) => !beforeIPs.has(ip)));
  const removedIPs = [...beforeIPs].filter((ip) => !afterIPs.has(ip));
  const dnsIPsChanged = addedIPs.size > 0 || removedIPs.length > 0;
  const sameDNS = !added.length && !removed.length && [...before].every(([key, n]) =>
    after.get(key)?.type === n.type && after.get(key)?.suspect === n.suspect);
  const dnsChanges: DNSChange[] = [];
  if (oldDNS.length && newDNS && removed.length === before.size && added.length === after.size) {
    dnsChanges.push({ removed: oldDNS, text: ` → ${newDNS}` });
  } else {
    if (oldDNS.length) {
      const base = cur ? c.removedCurrent : c.removedPrevious;
      const space = locale === 'zh-CN' && /^[A-Za-z0-9]/.test(oldDNS[0].name) ? ' ' : '';
      dnsChanges.push({ prefix: `${base}${space}`, removed: oldDNS });
    }
    if (newDNS) {
      const base = cur ? c.addedCurrent : c.addedPrevious;
      const space = locale === 'zh-CN' && /^[A-Za-z0-9]/.test(newDNS) ? ' ' : '';
      dnsChanges.push({ text: `${base}${space}${newDNS}` });
    }
  }
  if (!dnsChanges.length && !sameDNS) dnsChanges.push({ text: c.attributionChanged });
  if (!dnsChanges.length && dnsIPsChanged) dnsChanges.push({ text: c.ipsOnlyChanged });

  const oldExit = previous.exit;
  const newExit = current.exit;
  let exitChange = '';
  if (!oldExit?.ip || !newExit?.ip) {
    exitChange = !oldExit?.ip && !newExit?.ip
      ? c.bothMissing
      : !oldExit?.ip
        ? c.prevMissing
        : cur
          ? c.curMissingCurrent
          : c.curMissingPrevious;
  } else if (exitLabel(oldExit, locale) !== exitLabel(newExit, locale)) {
    exitChange = `${exitLabel(oldExit, locale)} → ${exitLabel(newExit, locale)}`;
  } else if (oldExit.ip !== newExit.ip) {
    exitChange = c.exitIPChanged;
  } else if (oldExit.asn !== newExit.asn || dnsExitType(oldExit) !== dnsExitType(newExit)) {
    exitChange = c.attributionChanged;
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
    summary: stable ? (dnsIPsChanged ? c.sameProvidersNewIPs : c.identical) : '',
    dnsChanges, exitChange, addedIPs, removedIPs,
  };
}
