import type { FPJSAudit, FingerprintHookAudit } from '../types';

function isNativeFunction(fn: Function): boolean {
  try {
    return Function.prototype.toString.call(fn).includes('[native code]');
  } catch {
    return false;
  }
}

function auditEnvironment(): FingerprintHookAudit {
  const suspectMethods: Function[] = [
    HTMLCanvasElement.prototype.toDataURL,
    CanvasRenderingContext2D.prototype.getImageData,
    CanvasRenderingContext2D.prototype.measureText,
    Function.prototype.toString
  ];

  const hookedMethods: string[] = [];

  suspectMethods.forEach(fn => {
    if (!isNativeFunction(fn)) {
      hookedMethods.push(fn.name || 'anonymous');
    }
  });

  if (hookedMethods.length > 0) {
    return {
      status: 'DANGER',
      message: `代码注入: ${hookedMethods.length} 个 API 被篡改`,
      methods: hookedMethods
    };
  }

  return { status: 'SAFE', message: '原生环境 (未检测到 Hook)', methods: [] };
}

export async function collectFingerprintAudit(): Promise<FPJSAudit> {
  if (typeof window === 'undefined') {
    return {
      hook: {
        status: 'DANGER',
        message: 'not in browser',
        methods: []
      }
    };
  }

  return {
    hook: auditEnvironment()
  };
}
