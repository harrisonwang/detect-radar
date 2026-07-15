/**
 * 泄漏检测采集器 (Leaks)
 * 检测隐私泄漏风险：WebRTC / IPv6（客户端实时）+ DNS（需后端配合，单独调用）
 */

import type { LeaksResults } from '../types';
import { gatherRTCCandidates, toWebRTCLeakResult } from './webrtc';
import { toIPv6LeakResult } from './ipv6';

/**
 * 采集客户端可独立完成的泄露信号（WebRTC + IPv6）。
 * 二者共享一次 ICE 收集，随主扫描一并提交给 /scans。
 * DNS 泄露需要与后端权威 NS 往返，耗时且依赖基础设施，故不放在此处，
 * 由上层在主扫描后以 collectDNSLeak() 单独增量执行。
 */
export const collectLeaks = async (): Promise<LeaksResults> => {
  const gather = await gatherRTCCandidates();
  return {
    webrtc: toWebRTCLeakResult(gather),
    ipv6: toIPv6LeakResult(gather),
  };
};

// 导出单个采集器
export { collectWebRTCLeak, gatherRTCCandidates } from './webrtc';
export { collectDNSLeak } from './dns';
