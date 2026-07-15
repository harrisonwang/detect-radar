/**
 * 采集器统一入口
 * 按功能域分组：Fingerprint / Environment / Leaks
 */

import { collectFingerprints } from './fingerprint';
import { collectEnvironment } from './environment';
import { collectLeaks } from './leaks';
import type { AllCollectorResults } from './types';

/**
 * 采集所有数据
 */
export const collectAll = async (): Promise<AllCollectorResults> => {
  const [fingerprint, environment, leaks] = await Promise.all([
    collectFingerprints(),
    collectEnvironment(),
    collectLeaks(),
  ]);

  return { fingerprint, environment, leaks };
};

// ===== 按分组导出 =====
export { collectFingerprints } from './fingerprint';
export { collectEnvironment } from './environment';
export { collectLeaks } from './leaks';

// ===== 单个采集器导出 (便捷访问) =====
export { collectCanvas, collectAudio, collectWebGL } from './fingerprint';
export {
  collectNavigator,
  collectUserAgentData,
  collectScreen,
  collectTimezone
} from './environment';

// ===== 类型导出 =====
export * from './types';

// ===== 工具函数导出 =====
export { cyrb53, toHexHash } from './utils/hash';
