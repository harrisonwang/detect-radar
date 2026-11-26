
import type { CollectorResult, AudioFingerprintData } from '../types';

declare global {
  interface Window {
    webkitOfflineAudioContext?: typeof OfflineAudioContext;
  }
}

export const collectAudio = async (): Promise<CollectorResult<AudioFingerprintData>> => {
  const start = performance.now();

  try {
    const AudioContext = window.OfflineAudioContext || window.webkitOfflineAudioContext;
    if (!AudioContext) {
      return {
        status: 'error',
        error: 'OfflineAudioContext not supported',
        timing: performance.now() - start
      };
    }

    const nativeCode = AudioContext.toString();

    return {
      status: 'success',
      data: {
        native_code: nativeCode
      },
      timing: performance.now() - start
    };
  } catch (e) {
    return {
      status: 'error',
      error: (e as Error).message,
      timing: performance.now() - start
    };
  }
};
