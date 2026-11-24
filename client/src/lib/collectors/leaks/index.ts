/**
 * 泄漏检测采集器 (Leaks)
 * 检测隐私泄漏风险：WebRTC / IPv6（客户端实时）+ DNS（需后端配合，单独调用）
 */

import type { LeaksResults } from '../types';
import { gatherRTCCandidates, toWebRTCLeakResult } from './webrtc';
import { collectIPv6Leak } from './ipv6';

/**
 * 并发采集客户端可直接完成的泄露信号：
 * - WebRTC（本地 ICE/STUN 探测）
 * - IPv6（4.detectradar.com + 6.detectradar.com 独立双栈 HTTP 探测，不依赖 WebRTC）
 * 二者并发执行后随主扫描提交给 /scans；DNS 泄露走 collectDNSLeak() 异步富化。
 */
export const collectLeaks = async (): Promise<LeaksResults> => {
  const [gather, ipv6] = await Promise.all([
    gatherRTCCandidates(),
    collectIPv6Leak(),
  ]);
  return {
    webrtc: toWebRTCLeakResult(gather),
    ipv6,
  };
};

// 导出单个采集器
export { collectWebRTCLeak, gatherRTCCandidates } from './webrtc';
export { collectIPv6Leak } from './ipv6';
export { collectDNSLeak } from './dns';
