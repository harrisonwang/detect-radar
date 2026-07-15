/**
 * WebGL Fingerprint Collector
 * 包含两部分：
 * 1. Metadata - 硬件信息、扩展列表、能力参数
 * 2. Image - 渲染图像指纹 (捕捉 GPU 浮点运算和光栅化差异)
 */

import { cyrb53 } from '../utils/hash';
import type { CollectorResult, WebGLFingerprintData, WebGLImageFingerprint } from '../types';

// 扩展 WebGLRenderingContext 类型
interface WEBGL_debug_renderer_info {
  UNMASKED_VENDOR_WEBGL: number;
  UNMASKED_RENDERER_WEBGL: number;
}

interface EXT_texture_filter_anisotropic {
  MAX_TEXTURE_MAX_ANISOTROPY_EXT: number;
}

// Shader 源码
const VERTEX_SHADER_SOURCE = `
  attribute vec2 position;
  varying vec2 vColor;
  void main() {
    gl_Position = vec4(position, 0.0, 1.0);
    vColor = (position + 1.0) * 0.5;
  }
`;

const FRAGMENT_SHADER_SOURCE = `
  precision mediump float;
  varying vec2 vColor;
  void main() {
    float r = vColor.x;
    float g = vColor.y;
    float b = abs(sin(gl_FragCoord.x * 0.1) + cos(gl_FragCoord.y * 0.1)) * 0.5;
    gl_FragColor = vec4(r, g, b, 1.0);
  }
`;

/**
 * 创建并编译 Shader
 */
const createShader = (
  gl: WebGLRenderingContext,
  type: number,
  source: string
): WebGLShader => {
  const shader = gl.createShader(type);
  if (!shader) throw new Error('Failed to create shader');

  gl.shaderSource(shader, source);
  gl.compileShader(shader);

  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const info = gl.getShaderInfoLog(shader);
    gl.deleteShader(shader);
    throw new Error(`Shader compile error: ${info}`);
  }

  return shader;
};

/**
 * 采集 WebGL 图像指纹
 * 通过 Shader 绘制渐变图形，读取像素生成哈希
 */
const collectWebGLImage = (gl: WebGLRenderingContext): WebGLImageFingerprint => {
  // 编译 Shader
  const vShader = createShader(gl, gl.VERTEX_SHADER, VERTEX_SHADER_SOURCE);
  const fShader = createShader(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER_SOURCE);

  const program = gl.createProgram();
  if (!program) throw new Error('Failed to create program');

  gl.attachShader(program, vShader);
  gl.attachShader(program, fShader);
  gl.linkProgram(program);

  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error('Program link error');
  }

  gl.useProgram(program);

  // 绘制覆盖整个 Canvas 的矩形
  const vertices = new Float32Array([
    -1.0, -1.0,
     1.0, -1.0,
    -1.0,  1.0,
    -1.0,  1.0,
     1.0, -1.0,
     1.0,  1.0
  ]);

  const buffer = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
  gl.bufferData(gl.ARRAY_BUFFER, vertices, gl.STATIC_DRAW);

  const positionLocation = gl.getAttribLocation(program, 'position');
  gl.enableVertexAttribArray(positionLocation);
  gl.vertexAttribPointer(positionLocation, 2, gl.FLOAT, false, 0, 0);

  gl.clearColor(0.0, 0.0, 0.0, 1.0);
  gl.clear(gl.COLOR_BUFFER_BIT);
  gl.drawArrays(gl.TRIANGLES, 0, 6);

  // 读取像素
  const width = gl.drawingBufferWidth;
  const height = gl.drawingBufferHeight;
  const pixels = new Uint8Array(width * height * 4);
  gl.readPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, pixels);

  // 计算哈希
  const pixelsStr = String.fromCharCode.apply(null, Array.from(pixels.slice(0, 1000)));
  const hash = cyrb53(pixelsStr).toString(16);

  // 清理
  gl.deleteShader(vShader);
  gl.deleteShader(fShader);
  gl.deleteProgram(program);
  gl.deleteBuffer(buffer);

  return {
    hash,
    pixels_length: pixels.length
  };
};

/**
 * 采集 WebGL 指纹 (Metadata + Image)
 */
export const collectWebGL = async (): Promise<CollectorResult<WebGLFingerprintData>> => {
  const start = performance.now();

  try {
    const canvas = document.createElement('canvas');
    canvas.width = 50;
    canvas.height = 50;

    let gl: WebGLRenderingContext | null = null;
    let contextName: string | null = null;

    // 尝试获取 WebGL 上下文
    const contextNames = ['webgl', 'experimental-webgl'] as const;
    for (const name of contextNames) {
      try {
        gl = canvas.getContext(name) as WebGLRenderingContext | null;
        if (gl) {
          contextName = name;
          break;
        }
      } catch {
        // continue
      }
    }

    if (!gl || !contextName) {
      return {
        status: 'error',
        error: 'WebGL context creation failed',
        timing: performance.now() - start
      };
    }

    // === Metadata 采集 ===

    // 基础信息
    const vendor = gl.getParameter(gl.VENDOR) as string | null;
    const renderer = gl.getParameter(gl.RENDERER) as string | null;

    // Debug 信息 (真实硬件)
    let unmaskedVendor: string | null = null;
    let unmaskedRenderer: string | null = null;
    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info') as WEBGL_debug_renderer_info | null;

    if (debugInfo) {
      unmaskedVendor = gl.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL) as string | null;
      unmaskedRenderer = gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) as string | null;
    }

    // 扩展列表
    const extensions = gl.getSupportedExtensions() || [];
    const extensionsHash = cyrb53(extensions.join(',')).toString(16);

    // 硬件能力
    const maxTextureSize = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;

    let maxAnisotropy = 0;
    const extAnisotropy = (
      gl.getExtension('EXT_texture_filter_anisotropic') ||
      gl.getExtension('MOZ_EXT_texture_filter_anisotropic') ||
      gl.getExtension('WEBKIT_EXT_texture_filter_anisotropic')
    ) as EXT_texture_filter_anisotropic | null;

    if (extAnisotropy) {
      maxAnisotropy = gl.getParameter(extAnisotropy.MAX_TEXTURE_MAX_ANISOTROPY_EXT) as number;
    }

    // === Image 采集 ===
    const image = collectWebGLImage(gl);

    // Native code (Hook 检测)
    const nativeCode = gl.readPixels.toString();

    return {
      status: 'success',
      data: {
        context_name: contextName,
        base_vendor: vendor,
        base_renderer: renderer,
        unmasked_vendor: unmaskedVendor,
        unmasked_renderer: unmaskedRenderer,
        extensions_hash: extensionsHash,
        extensions_count: extensions.length,
        capabilities: {
          max_texture_size: maxTextureSize,
          max_anisotropy: maxAnisotropy
        },
        image,
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
