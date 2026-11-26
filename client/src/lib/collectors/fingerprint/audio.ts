
import { cyrb53 } from '../utils/hash';
import type { CollectorResult, AudioFingerprintData } from '../types';

declare global {
  interface Window {
    webkitOfflineAudioContext?: typeof OfflineAudioContext;
  }
}

const generateAudioProfile = async (
  AudioContextClass: typeof OfflineAudioContext
): Promise<number> => {
  const context = new AudioContextClass(1, 44100, 44100);

  const oscillator = context.createOscillator();
  oscillator.type = 'triangle';
  oscillator.frequency.setValueAtTime(10000, context.currentTime);

  const compressor = context.createDynamicsCompressor();
  compressor.threshold.setValueAtTime(-50, context.currentTime);
  compressor.knee.setValueAtTime(40, context.currentTime);
  compressor.ratio.setValueAtTime(12, context.currentTime);
  compressor.attack.setValueAtTime(0, context.currentTime);
  compressor.release.setValueAtTime(0.25, context.currentTime);

  oscillator.connect(compressor);
  compressor.connect(context.destination);
  oscillator.start(0);

  const buffer = await context.startRendering();
  const data = buffer.getChannelData(0);

  let sum = 0;
  for (let i = 4500; i < 5000; i++) {
    sum += Math.abs(data[i]);
  }

  return sum;
};

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

    const entropy = await generateAudioProfile(AudioContext);
    const hash = cyrb53(entropy.toString()).toString(16);

    const nativeCode = AudioContext.toString();

    return {
      status: 'success',
      data: {
        hash_sample: hash,
        entropy_raw: entropy,
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
