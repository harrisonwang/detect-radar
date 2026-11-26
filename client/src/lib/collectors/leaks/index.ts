
import type { Locale } from '../../../i18n';
import type { LeaksResults } from '../types';
import { gatherRTCCandidates, toWebRTCLeakResult } from './webrtc';
import { collectIPv6Leak } from './ipv6';

export const collectLeaks = async (locale?: Locale): Promise<LeaksResults> => {
  const [gather, ipv6] = await Promise.all([
    gatherRTCCandidates(),
    collectIPv6Leak(locale),
  ]);
  return {
    webrtc: toWebRTCLeakResult(gather),
    ipv6,
  };
};

export { gatherRTCCandidates } from './webrtc';
export { collectIPv6Leak } from './ipv6';
export { collectDNSLeak } from './dns';
