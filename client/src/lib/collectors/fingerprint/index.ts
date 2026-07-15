/**
 * 指纹采集器 (Fingerprint)
 * 硬件/渲染层面的底层特征，难以伪造
 */

import { collectCanvas } from './canvas';
import { collectAudio } from './audio';
import { collectWebGL } from './webgl';
import type { FingerprintResults } from '../types';

/**
 * 并行采集所有指纹
 */
export const collectFingerprints = async (): Promise<FingerprintResults> => {
  const [canvas, audio, webgl] = await Promise.all([
    collectCanvas(),
    collectAudio(),
    collectWebGL(),
  ]);

  return { canvas, audio, webgl };
};

// 导出单个采集器
export { collectCanvas } from './canvas';
export { collectAudio } from './audio';
export { collectWebGL } from './webgl';
