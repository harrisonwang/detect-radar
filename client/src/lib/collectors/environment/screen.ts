/**
 * Screen Info Collector
 * 采集屏幕/窗口信息
 * 用于检测 mobile UA + desktop 分辨率 的矛盾
 */

import type { CollectorResult, ScreenData } from '../types';

/**
 * 采集屏幕信息
 */
export const collectScreen = async (): Promise<CollectorResult<ScreenData>> => {
  const start = performance.now();

  try {
    const scr = window.screen;

    return {
      status: 'success',
      data: {
        width: scr.width,
        height: scr.height,
        availWidth: scr.availWidth,
        availHeight: scr.availHeight,
        colorDepth: scr.colorDepth,
        pixelDepth: scr.pixelDepth,
        devicePixelRatio: window.devicePixelRatio || 1
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
