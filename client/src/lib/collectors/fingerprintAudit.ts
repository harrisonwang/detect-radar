import type { FingerprintAuditPayload, FingerprintHookAudit } from '../types';

const FPJS_CDN = 'https://openfpcdn.io/fingerprintjs/v4';

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

  try {
    if (!isNativeFunction(HTMLCanvasElement.prototype.toDataURL)) {
      hookedMethods.push('toDataURL(DeepCheck)');
    }
  } catch {
    // ignore
  }

  if (hookedMethods.length > 0) {
    return {
      status: 'DANGER',
      message: `代码注入: ${hookedMethods.length} 个 API 被篡改`,
      methods: hookedMethods
    };
  }

  return { status: 'SAFE', message: '原生环境 (未检测到 Hook)', methods: [] };
}

async function loadFingerprintJS() {
  // FPJS_CDN 是运行时从 CDN 动态加载的外部 ESM URL，故意不打包；@vite-ignore 关掉“无法静态分析”的告警
  const module = await import(/* @vite-ignore */ FPJS_CDN);
  return module.default ?? module;
}

function summarizeCanvasComponent(component: unknown): string | undefined {
  if (!component) {
    return undefined;
  }
  try {
    const canvasStr = JSON.stringify(component);
    const snippet = canvasStr.slice(0, 150);
    return `${snippet}${canvasStr.length > 150 ? '... ' : ' '}(${canvasStr.length})`;
  } catch {
    return 'canvas component stringify failed';
  }
}

export async function collectFingerprintAudit(): Promise<FingerprintAuditPayload> {
  if (typeof window === 'undefined') {
    return {
      hook: {
        status: 'DANGER',
        message: 'not in browser',
        methods: []
      }
    };
  }

  const hookAudit = auditEnvironment();
  let visitorId: string | undefined;
  let canvasSummary: string | undefined;

  try {
    const FingerprintJS = await loadFingerprintJS();
    const fp = await FingerprintJS.load();
    const result = await fp.get();
    visitorId = result?.visitorId;
    canvasSummary = summarizeCanvasComponent(result?.components?.canvas?.value);
  } catch {
    // ignore, keep undefined
  }

  return {
    visitorId,
    canvasSummary,
    hook: hookAudit
  };
}
