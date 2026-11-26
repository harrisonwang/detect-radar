
import { toHexHash } from '../utils/hash';
import type { CollectorResult, CanvasFingerprintData } from '../types';

const drawCanvasFeatures = (canvas: HTMLCanvasElement): void => {
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  ctx.textBaseline = 'alphabetic';

  ctx.fillStyle = '#f60';
  ctx.fillRect(125, 1, 62, 20);

  ctx.globalCompositeOperation = 'multiply';
  ctx.fillStyle = '#069';
  ctx.fillRect(125, 1, 62, 20);
  ctx.globalCompositeOperation = 'source-over';

  ctx.font = '14px Arial';
  ctx.fillStyle = '#069';
  ctx.fillText('DetectRadar', 2, 15);

  ctx.fillStyle = 'rgba(0,0,0,1)';
  ctx.font = '16pt Arial';
  ctx.fillText('Cwm fjord \ud83d\ude03', 2, 45);
};

export const collectCanvas = async (): Promise<CollectorResult<CanvasFingerprintData>> => {
  const start = performance.now();

  try {
    const c1 = document.createElement('canvas');
    c1.width = 240;
    c1.height = 80;
    drawCanvasFeatures(c1);
    const data1 = c1.toDataURL();
    const hash1 = toHexHash(data1);

    const c2 = document.createElement('canvas');
    c2.width = 240;
    c2.height = 80;
    drawCanvasFeatures(c2);
    const data2 = c2.toDataURL();
    const hash2 = toHexHash(data2);

    const nativeCode = HTMLCanvasElement.prototype.toDataURL.toString();

    return {
      status: 'success',
      data: {
        hash_sample_a: hash1,
        hash_sample_b: hash2,
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
