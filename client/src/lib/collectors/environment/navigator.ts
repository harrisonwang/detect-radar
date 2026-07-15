/**
 * Navigator Info Collector
 * 采集浏览器/设备基本信息
 *
 * 注意：platform 虽然被 MDN 标记为 Deprecated，但在指纹检测场景应保留：
 * 1. 用于检测 platform 与 userAgent 的一致性（指纹浏览器常忘记同步修改）
 * 2. 空值本身也是有意义的信号（现代浏览器返回空，但 userAgent 仍有系统信息 → 异常）
 */

import type { CollectorResult, NavigatorData } from '../types';

// 扩展 Navigator 类型
type ExtendedNavigator = Navigator & {
  deviceMemory?: number;
};

/**
 * 采集 Navigator 信息
 */
export const collectNavigator = async (): Promise<CollectorResult<NavigatorData>> => {
  const start = performance.now();

  try {
    const nav = window.navigator as ExtendedNavigator;

    return {
      status: 'success',
      data: {
        userAgent: nav.userAgent,
        platform: nav.platform,
        language: nav.language,
        languages: Array.from(nav.languages || []),
        hardwareConcurrency: nav.hardwareConcurrency || 0,
        deviceMemory: nav.deviceMemory ?? null,
        maxTouchPoints: nav.maxTouchPoints || 0,
        cookieEnabled: nav.cookieEnabled,
        doNotTrack: nav.doNotTrack,
        // 新增字段
        webdriver: nav.webdriver,           // 🔴 红线：自动化浏览器标记
        vendor: nav.vendor || '',           // 浏览器供应商
        plugins_length: nav.plugins?.length ?? 0  // iOS 应为 0
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
