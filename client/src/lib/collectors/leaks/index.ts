
import type { LeaksResults } from '../types';
import { gatherRTCCandidates, toWebRTCLeakResult } from './webrtc';
import { collectIPv6Leak } from './ipv6';

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

export { gatherRTCCandidates } from './webrtc';
export { collectIPv6Leak } from './ipv6';
export { collectDNSLeak } from './dns';
