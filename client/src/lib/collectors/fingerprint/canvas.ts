/**
 * Canvas Fingerprint Collector
 * 策略：采集两次 -> 生成两个 Hash -> 全部上报，由后端判断稳定性
 */

import { toHexHash } from '../utils/hash';
import type { CollectorResult, CanvasFingerprintData } from '../types';

/**
 * 绘制 Canvas 特征图案
 * 制造复杂的渲染场景以生成唯一指纹
 */
const drawCanvasFeatures = (canvas: HTMLCanvasElement): void => {
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  ctx.textBaseline = 'alphabetic';

  // 基础矩形
  ctx.fillStyle = '#f60';
  ctx.fillRect(125, 1, 62, 20);

  // 混合模式与渐变
  ctx.globalCompositeOperation = 'multiply';
  ctx.fillStyle = '#069';
  ctx.fillRect(125, 1, 62, 20);
  ctx.globalCompositeOperation = 'source-over';

  // 文本渲染
  ctx.font = '14px Arial';
  ctx.fillStyle = '#069';
  ctx.fillText('DetectRadar', 2, 15);

  // Emoji 渲染 (不同系统渲染结果不同)
  ctx.fillStyle = 'rgba(0,0,0,1)';
  ctx.font = '16pt Arial';
  ctx.fillText('Cwm fjord \ud83d\ude03', 2, 45);
};

/**
 * 采集 Canvas 指纹
 * @returns 采集结果，包含两个样本的哈希值
 */
export const collectCanvas = async (): Promise<CollectorResult<CanvasFingerprintData>> => {
  const start = performance.now();

  try {
    // 第一次采集
    const c1 = document.createElement('canvas');
    c1.width = 240;
    c1.height = 80;
    drawCanvasFeatures(c1);
    const data1 = c1.toDataURL();
    const hash1 = toHexHash(data1);

    // 第二次采集 (用于稳定性检测)
    const c2 = document.createElement('canvas');
    c2.width = 240;
    c2.height = 80;
    drawCanvasFeatures(c2);
    const data2 = c2.toDataURL();
    const hash2 = toHexHash(data2);

    // 获取原生代码字符串 (用于 Hook 检测)
    const nativeCode = HTMLCanvasElement.prototype.toDataURL.toString();

    return {
      status: 'success',
      data: {
        hash_sample_a: hash1,
        hash_sample_b: hash2,
        native_code: nativeCode,
        data_length: data1.length
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
