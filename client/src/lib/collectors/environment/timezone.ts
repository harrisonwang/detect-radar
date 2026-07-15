/**
 * Timezone Collector
 * 采集时区信息
 */

import type { CollectorResult, TimezoneData } from '../types';

/**
 * 采集时区信息
 */
export const collectTimezone = async (): Promise<CollectorResult<TimezoneData>> => {
  const start = performance.now();

  try {
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const timezoneOffset = new Date().getTimezoneOffset();

    return {
      status: 'success',
      data: {
        timezone,         // e.g. "Asia/Shanghai"
        timezoneOffset    // e.g. -480 (分钟，UTC+8 = -480)
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
