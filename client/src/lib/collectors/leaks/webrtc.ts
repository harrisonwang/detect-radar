
import type { CollectorResult, WebRTCLeakData } from '../types';

const STUN_SERVERS = [
  'stun:stun.l.google.com:19302',
  'stun:stun1.l.google.com:19302',
];

const GATHER_TIMEOUT_MS = 2500;

export type RTCCandidateType = 'host' | 'srflx' | 'relay' | 'prflx' | 'unknown';
export type RTCLocalScope = 'lan' | 'cgnat' | 'fakeip' | 'link-local' | 'ula' | 'loopback';

export interface RTCCandidateDetail {
  address: string;
  kind: 'public' | 'local' | 'mdns';
  family: 'ipv4' | 'ipv6' | 'mdns';
  type: RTCCandidateType;
  protocol: string;
  localScope?: RTCLocalScope;
}

function isIPv4(s: string): boolean {
  const p = s.split('.');
  if (p.length !== 4) return false;
  return p.every((o) => /^\d{1,3}$/.test(o) && Number(o) <= 255);
}

function isIPv6(s: string): boolean {
  if ((s.match(/:/g)?.length ?? 0) < 2) return false;
  if (!/^[0-9a-fA-F:]+$/.test(s)) return false;
  return /^([0-9a-fA-F]{1,4})?(:[0-9a-fA-F]{0,4}){2,}$/.test(s);
}

function isMDNSHost(s: string): boolean {
  const clean = s.replace(/\.$/, '');
  return clean.toLowerCase().endsWith('.local') && /^[a-zA-Z0-9.-]+$/.test(clean);
}

export interface RTCGatherResult {
  apiSupported: boolean;
  localIPs: string[];
  publicIPs: string[];
  stunIPs: string[];
  mdnsHosts: string[];
  ipv6Global: string | null;
  candidates: RTCCandidateDetail[];
}

export function classifyLocalScope(ip: string): RTCLocalScope | undefined {
  if (ip.includes(':')) {
    const l = ip.toLowerCase();
    if (l === '::1') return 'loopback';
    if (/^fe[89ab][0-9a-f]:/.test(l)) return 'link-local';
    if (l.startsWith('fc') || l.startsWith('fd')) return 'ula';
    return undefined;
  }
  const p = ip.split('.').map(Number);
  if (p.length !== 4 || p.some((n) => Number.isNaN(n))) return undefined;
  if (p[0] === 0 || p[0] === 127) return 'loopback';
  if (p[0] === 198 && (p[1] === 18 || p[1] === 19)) return 'fakeip';
  if (p[0] === 100 && p[1] >= 64 && p[1] <= 127) return 'cgnat';
  if (p[0] === 169 && p[1] === 254) return 'link-local';
  if (
    p[0] === 10 ||
    (p[0] === 172 && p[1] >= 16 && p[1] <= 31) ||
    (p[0] === 192 && p[1] === 168)
  ) {
    return 'lan';
  }
  return undefined;
}

export function isPrivateIPv4(ip: string): boolean {
  return !ip.includes(':') && classifyLocalScope(ip) !== undefined;
}

export function isPrivateIPv6(ip: string): boolean {
  return ip.includes(':') && classifyLocalScope(ip) !== undefined;
}

function parseCandidateType(parts: string[]): RTCCandidateType {
  const typIdx = parts.indexOf('typ');
  const raw = typIdx >= 0 ? (parts[typIdx + 1] ?? '').toLowerCase() : '';
  if (raw === 'host' || raw === 'srflx' || raw === 'relay' || raw === 'prflx') {
    return raw;
  }
  return 'unknown';
}

function extractIP(parts: string[]): string | null {
  const tok = parts[4];
  if (!tok) return null;
  if (isIPv4(tok) || isIPv6(tok)) return tok;
  return null;
}

export async function gatherRTCCandidates(
  onUpdate?: (snapshot: RTCGatherResult) => void,
): Promise<RTCGatherResult> {
  const apiSupported = typeof RTCPeerConnection !== 'undefined';
  const result: RTCGatherResult = {
    apiSupported,
    localIPs: [],
    publicIPs: [],
    stunIPs: [],
    mdnsHosts: [],
    ipv6Global: null,
    candidates: [],
  };

  if (!apiSupported) {
    return result;
  }

  const local = new Set<string>();
  const pub = new Set<string>();
  const stun = new Set<string>();
  const mdns = new Set<string>();
  const seenCandidates = new Set<string>();
  const details: RTCCandidateDetail[] = [];

  const pc = new RTCPeerConnection({
    iceServers: STUN_SERVERS.map((urls) => ({ urls })),
  });

  const emitUpdate = () => {
    onUpdate?.({
      apiSupported,
      localIPs: [...local],
      publicIPs: [...pub],
      stunIPs: [...stun],
      mdnsHosts: [...mdns],
      ipv6Global: result.ipv6Global,
      candidates: [...details],
    });
  };

  const pushCandidate = (detail: RTCCandidateDetail) => {
    const key = `${detail.address}|${detail.type}|${detail.protocol}`;
    if (seenCandidates.has(key)) return;
    seenCandidates.add(key);
    details.push(detail);
    emitUpdate();
  };

  const classify = (candidate: string) => {
    const parts = candidate.trim().split(/\s+/);
    const tok = parts[4];
    if (!tok) return;

    const protocol = (parts[2] || 'udp').toLowerCase();
    const type = parseCandidateType(parts);

    if (isMDNSHost(tok)) {
      const clean = tok.replace(/\.$/, '').toLowerCase();
      mdns.add(clean);
      pushCandidate({
        address: clean,
        kind: 'mdns',
        family: 'mdns',
        type,
        protocol,
      });
      return;
    }

    const ip = extractIP(parts);
    if (!ip) return;

    const isV6 = ip.includes(':');
    const family: 'ipv4' | 'ipv6' = isV6 ? 'ipv6' : 'ipv4';
    const isSrflx = type === 'srflx' || type === 'relay';
    const localScope = classifyLocalScope(ip);

    if (localScope) {
      local.add(ip);
      pushCandidate({
        address: ip,
        kind: 'local',
        family,
        type,
        protocol,
        localScope,
      });
      return;
    }

    pub.add(ip);
    if (isSrflx) stun.add(ip);
    if (isV6 && !result.ipv6Global) result.ipv6Global = ip;
    pushCandidate({
      address: ip,
      kind: 'public',
      family,
      type,
      protocol,
    });
  };

  try {
    pc.createDataChannel('detectradar');

    await new Promise<void>((resolve) => {
      let done = false;
      const finish = () => {
        if (done) return;
        done = true;
        resolve();
      };

      pc.onicecandidate = (e) => {
        if (!e.candidate) {
          finish();
          return;
        }
        if (e.candidate.candidate) classify(e.candidate.candidate);
      };
      pc.onicegatheringstatechange = () => {
        if (pc.iceGatheringState === 'complete') finish();
      };

      pc.createOffer()
        .then((offer) => pc.setLocalDescription(offer))
        .catch(finish);

      setTimeout(finish, GATHER_TIMEOUT_MS);
    });
  } catch {
  } finally {
    try {
      pc.close();
    } catch {
    }
  }

  result.localIPs = [...local];
  result.publicIPs = [...pub];
  result.stunIPs = [...stun];
  result.mdnsHosts = [...mdns];
  result.candidates = details;
  return result;
}

export function toWebRTCLeakResult(g: RTCGatherResult): CollectorResult<WebRTCLeakData> {
  if (!g.apiSupported) {
    return { status: 'unsupported', error: 'RTCPeerConnection 不可用' };
  }
  return {
    status: 'success',
    data: {
      leaked: g.publicIPs.length > 0,
      local_ips: g.localIPs,
      public_ips: g.publicIPs,
      stun_ips: g.stunIPs,
    },
  };
}

export const collectWebRTCLeak = async (): Promise<CollectorResult<WebRTCLeakData>> =>
  toWebRTCLeakResult(await gatherRTCCandidates());
