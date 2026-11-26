
import type { CollectorResult, WebGLFingerprintData } from '../types';

interface WEBGL_debug_renderer_info {
  UNMASKED_RENDERER_WEBGL: number;
}

export const collectWebGL = async (): Promise<CollectorResult<WebGLFingerprintData>> => {
  const start = performance.now();

  try {
    const canvas = document.createElement('canvas');
    canvas.width = 50;
    canvas.height = 50;

    let gl: WebGLRenderingContext | null = null;
    const contextNames = ['webgl', 'experimental-webgl'] as const;
    for (const name of contextNames) {
      try {
        gl = canvas.getContext(name) as WebGLRenderingContext | null;
        if (gl) break;
      } catch {
      }
    }

    if (!gl) {
      return {
        status: 'error',
        error: 'WebGL context creation failed',
        timing: performance.now() - start
      };
    }

    let unmaskedRenderer: string | null = null;
    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info') as WEBGL_debug_renderer_info | null;
    if (debugInfo) {
      unmaskedRenderer = gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) as string | null;
    }

    return {
      status: 'success',
      data: {
        unmasked_renderer: unmaskedRenderer
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
