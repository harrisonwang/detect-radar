/**
 * Audio Fingerprint Collector
 * 策略：只采集一次 (指纹浏览器通常会缓存音频指纹，无需重复采集)
 */

import { cyrb53 } from '../utils/hash';
import type { CollectorResult, AudioFingerprintData } from '../types';

// 扩展 Window 接口以支持 webkit 前缀
declare global {
  interface Window {
    webkitOfflineAudioContext?: typeof OfflineAudioContext;
  }
}

/**
 * 生成音频特征数据
 * 通过振荡器 + 动态压缩器模拟复杂的音频场景
 * @param AudioContextClass OfflineAudioContext 构造函数
 * @returns 音频熵值 (浮点数)
 */
const generateAudioProfile = async (
  AudioContextClass: typeof OfflineAudioContext
): Promise<number> => {
  const context = new AudioContextClass(1, 44100, 44100);

  // 创建振荡器
  const oscillator = context.createOscillator();
  oscillator.type = 'triangle';
  oscillator.frequency.setValueAtTime(10000, context.currentTime);

  // 创建动态压缩器 - 差异化的核心来源
  const compressor = context.createDynamicsCompressor();
  compressor.threshold.setValueAtTime(-50, context.currentTime);
  compressor.knee.setValueAtTime(40, context.currentTime);
  compressor.ratio.setValueAtTime(12, context.currentTime);
  compressor.attack.setValueAtTime(0, context.currentTime);
  compressor.release.setValueAtTime(0.25, context.currentTime);

  // 连接并启动
  oscillator.connect(compressor);
  compressor.connect(context.destination);
  oscillator.start(0);

  // 渲染音频
  const buffer = await context.startRendering();
  const data = buffer.getChannelData(0);

  // 计算特征值 (Entropy)
  // 选取 4500-5000 这一段数据进行求和，既快又能体现压缩特性
  let sum = 0;
  for (let i = 4500; i < 5000; i++) {
    sum += Math.abs(data[i]);
  }

  return sum;
};

/**
 * 采集 Audio 指纹
 * @returns 采集结果
 */
export const collectAudio = async (): Promise<CollectorResult<AudioFingerprintData>> => {
  const start = performance.now();

  try {
    // 兼容性检查
    const AudioContext = window.OfflineAudioContext || window.webkitOfflineAudioContext;
    if (!AudioContext) {
      return {
        status: 'error',
        error: 'OfflineAudioContext not supported',
        timing: performance.now() - start
      };
    }

    // 采集
    const entropy = await generateAudioProfile(AudioContext);
    const hash = cyrb53(entropy.toString()).toString(16);

    // 获取原生代码字符串 (用于 Hook 检测)
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
