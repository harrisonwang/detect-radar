
import type { CollectorResult, NavigatorData } from '../types';

type ExtendedNavigator = Navigator & {
  deviceMemory?: number;
};

export const collectNavigator = async (): Promise<CollectorResult<NavigatorData>> => {
  const start = performance.now();

  try {
    const nav = window.navigator as ExtendedNavigator;

    return {
      status: 'success',
      data: {
        platform: nav.platform,
        language: nav.language,
        languages: Array.from(nav.languages || []),
        hardwareConcurrency: nav.hardwareConcurrency || 0,
        deviceMemory: nav.deviceMemory ?? null,
        maxTouchPoints: nav.maxTouchPoints || 0,
        webdriver: nav.webdriver,
        plugins_length: nav.plugins?.length ?? 0
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
