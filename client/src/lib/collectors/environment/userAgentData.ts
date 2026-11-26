
import type { CollectorResult, UserAgentDataInfo } from '../types';

interface NavigatorUAData {
  mobile: boolean;
}

interface ExtendedNavigator extends Navigator {
  userAgentData?: NavigatorUAData;
}

export const collectUserAgentData = async (): Promise<CollectorResult<UserAgentDataInfo>> => {
  const start = performance.now();

  try {
    const nav = window.navigator as ExtendedNavigator;

    if (!nav.userAgentData) {
      return {
        status: 'unsupported',
        error: 'navigator.userAgentData not supported',
        timing: performance.now() - start
      };
    }

    return {
      status: 'success',
      data: {
        mobile: nav.userAgentData.mobile
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
