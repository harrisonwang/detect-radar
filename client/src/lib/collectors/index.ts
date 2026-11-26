
import { collectFingerprints } from './fingerprint';
import { collectEnvironment } from './environment';
import { collectLeaks } from './leaks';
import type { AllCollectorResults } from './types';

export const collectAll = async (): Promise<AllCollectorResults> => {
  const [fingerprint, environment, leaks] = await Promise.all([
    collectFingerprints(),
    collectEnvironment(),
    collectLeaks(),
  ]);

  return { fingerprint, environment, leaks };
};

export { collectFingerprints } from './fingerprint';
export { collectEnvironment } from './environment';
export { collectLeaks } from './leaks';

export { collectCanvas, collectAudio, collectWebGL } from './fingerprint';
export {
  collectNavigator,
  collectUserAgentData,
  collectScreen,
  collectTimezone
} from './environment';

export * from './types';

export { cyrb53, toHexHash } from './utils/hash';
