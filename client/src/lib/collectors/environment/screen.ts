
import type { CollectorResult, ScreenData } from '../types';

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
