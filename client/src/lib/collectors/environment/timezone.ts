
import type { CollectorResult, TimezoneData } from '../types';

export const collectTimezone = async (): Promise<CollectorResult<TimezoneData>> => {
  const start = performance.now();

  try {
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const timezoneOffset = new Date().getTimezoneOffset();

    return {
      status: 'success',
      data: {
        timezone,
        timezoneOffset
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
