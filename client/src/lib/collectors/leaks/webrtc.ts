/**
 * WebRTC 泄露采集器
 *
 * 通过 RTCPeerConnection 收集 ICE 候选，提取本机内网 IP 与 STUN 反射得到的公网 IP。
 * 判定「是否泄露」由服务端完成（公网候选 != 出口 IP 即泄露），客户端只如实上报候选。
 *
 * 现代浏览器默认用 mDNS(`*.local`) 隐藏内网 IP，因此 local_ips 可能为空或为 .local 主机名——
 * 这本身是隐私保护生效的表现，不作为泄露。
 */

import type { CollectorResult, WebRTCLeakData } from '../types';

const STUN_SERVERS = [
  'stun:stun.l.google.com:19302',
  'stun:stun1.l.google.com:19302',
];

const GATHER_TIMEOUT_MS = 2500;

// 校验 IPv4：四段 0-255
function isIPv4(s: string): boolean {
  const p = s.split('.');
  if (p.length !== 4) return false;
  return p.every((o) => /^\d{1,3}$/.test(o) && Number(o) <= 255);
}

// 校验 IPv6：至少两个冒号、仅含十六进制段（含 :: 缩写），排除把候选编号当地址
function isIPv6(s: string): boolean {
  if ((s.match(/:/g)?.length ?? 0) < 2) return false;
  if (!/^[0-9a-fA-F:]+$/.test(s)) return false;
  return /^([0-9a-fA-F]{1,4})?(:[0-9a-fA-F]{0,4}){2,}$/.test(s);
}

export interface RTCGatherResult {
  localIPs: string[]; // 私网 IPv4 / 链路本地
  publicIPs: string[]; // 公网 IPv4/IPv6（srflx 反射）
  stunIPs: string[]; // 来自 srflx（STUN 服务器观测到的地址）
  ipv6Global: string | null; // 命中的全局 IPv6，用于 IPv6 泄露判定
}

function isPrivateIPv4(ip: string): boolean {
  const p = ip.split('.').map(Number);
  if (p.length !== 4 || p.some((n) => Number.isNaN(n))) return false;
  return (
    p[0] === 10 ||
    (p[0] === 172 && p[1] >= 16 && p[1] <= 31) ||
    (p[0] === 192 && p[1] === 168) ||
    (p[0] === 169 && p[1] === 254) || // 链路本地
    p[0] === 127
  );
}

function isPrivateIPv6(ip: string): boolean {
  const l = ip.toLowerCase();
  return (
    l === '::1' ||
    l.startsWith('fe80') || // 链路本地
    l.startsWith('fc') ||
    l.startsWith('fd') // 唯一本地地址 ULA
  );
}

function extractIP(candidate: string): string | null {
  // RFC5245 candidate 属性：foundation component transport priority ADDRESS port typ ...
  // 地址固定在按空格切分后的第 5 段（index 4）。只认这一段并严格校验，
  // 不做正则全串兜底——否则会把 priority/foundation 这类大整数误当成 IP。
  const parts = candidate.trim().split(/\s+/);
  const tok = parts[4];
  if (!tok) return null;
  if (isIPv4(tok) || isIPv6(tok)) return tok;
  return null;
}

/**
 * 执行一次 ICE 收集，分类所有候选地址。
 * 单次 RTCPeerConnection 同时服务 WebRTC 与 IPv6 两个采集器，避免重复建连。
 */
export async function gatherRTCCandidates(): Promise<RTCGatherResult> {
  const result: RTCGatherResult = {
    localIPs: [],
    publicIPs: [],
    stunIPs: [],
    ipv6Global: null,
  };

  if (typeof RTCPeerConnection === 'undefined') {
    return result;
  }

  const local = new Set<string>();
  const pub = new Set<string>();
  const stun = new Set<string>();

  const pc = new RTCPeerConnection({
    iceServers: STUN_SERVERS.map((urls) => ({ urls })),
  });

  const classify = (candidate: string) => {
    const ip = extractIP(candidate);
    if (!ip || ip.endsWith('.local') || ip.includes('.local')) return;

    const isV6 = ip.includes(':');
    const isSrflx = candidate.includes('typ srflx') || candidate.includes('typ relay');
    const isPrivate = isV6 ? isPrivateIPv6(ip) : isPrivateIPv4(ip);

    if (isPrivate) {
      local.add(ip);
      return;
    }
    // 公网地址
    pub.add(ip);
    if (isSrflx) stun.add(ip);
    if (isV6 && !result.ipv6Global) result.ipv6Global = ip;
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
          finish(); // null candidate = 收集结束
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
    // 忽略，返回已收集到的部分
  } finally {
    try {
      pc.close();
    } catch {
      /* noop */
    }
  }

  result.localIPs = [...local];
  result.publicIPs = [...pub];
  result.stunIPs = [...stun];
  return result;
}

/** 将收集结果封装为 WebRTC 泄露采集结果 */
export function toWebRTCLeakResult(g: RTCGatherResult): CollectorResult<WebRTCLeakData> {
  if (typeof RTCPeerConnection === 'undefined') {
    return { status: 'unsupported', error: 'RTCPeerConnection 不可用' };
  }
  return {
    status: 'success',
    data: {
      // 是否真正泄露由服务端对比出口 IP 判定；此处标记「存在公网候选」供参考
      leaked: g.publicIPs.length > 0,
      local_ips: g.localIPs,
      public_ips: g.publicIPs,
      stun_ips: g.stunIPs,
    },
  };
}

/** 独立采集入口（如需单独测 WebRTC） */
export const collectWebRTCLeak = async (): Promise<CollectorResult<WebRTCLeakData>> =>
  toWebRTCLeakResult(await gatherRTCCandidates());
