
import { collectCanvas } from './canvas';
import { collectAudio } from './audio';
import { collectWebGL } from './webgl';
import type { FingerprintResults } from '../types';

export const collectFingerprints = async (): Promise<FingerprintResults> => {
  const [canvas, audio, webgl] = await Promise.all([
    collectCanvas(),
    collectAudio(),
    collectWebGL(),
  ]);

  return { canvas, audio, webgl };
};

export { collectCanvas } from './canvas';
export { collectAudio } from './audio';
export { collectWebGL } from './webgl';
