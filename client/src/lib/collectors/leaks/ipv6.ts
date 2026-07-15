/**
 * IPv6 泄露采集器
 *
 * 复用 WebRTC ICE 收集结果：若候选中出现「全局 IPv6 地址」，说明浏览器可经 IPv6
 * 直连、绕过仅代理 IPv4 的隧道，构成泄露风险。链路本地/ULA(fe80/fc/fd) 不算。
 *
 * 说明：纯前端无法主动向外发起 IPv6 探测（跨域受限），WebRTC 候选是浏览器内
 * 可靠且无需授权的 IPv6 暴露信号来源。
 */

import type { CollectorResult, IPv6LeakData } from '../types';
import type { RTCGatherResult } from './webrtc';

/** 从 WebRTC 收集结果推导 IPv6 泄露 */
export function toIPv6LeakResult(g: RTCGatherResult): CollectorResult<IPv6LeakData> {
  const address = g.ipv6Global;
  return {
    status: 'success',
    data: {
      leaked: address !== null,
      ipv6_address: address,
    },
  };
}
