
import type { CollectorResult, UserAgentDataInfo, UADataBrand } from '../types';

interface NavigatorUAData {
  brands: UADataBrand[];
  mobile: boolean;
  platform: string;
  getHighEntropyValues: (hints: string[]) => Promise<{
    architecture?: string;
    bitness?: string;
    model?: string;
    platformVersion?: string;
    fullVersionList?: UADataBrand[];
  }>;
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

    const uaData = nav.userAgentData;

    const baseData: UserAgentDataInfo = {
      brands: uaData.brands || [],
      mobile: uaData.mobile,
      platform: uaData.platform
    };

    try {
      const highEntropy = await uaData.getHighEntropyValues([
        'architecture',
        'bitness',
        'model',
        'platformVersion',
        'fullVersionList'
      ]);

      baseData.architecture = highEntropy.architecture;
      baseData.bitness = highEntropy.bitness;
      baseData.model = highEntropy.model;
      baseData.platformVersion = highEntropy.platformVersion;
      baseData.fullVersionList = highEntropy.fullVersionList;
    } catch {
    }

    return {
      status: 'success',
      data: baseData,
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
