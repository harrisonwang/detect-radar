/**
 * UserAgentData Collector
 * 采集现代浏览器的结构化 UA 信息 (navigator.userAgentData)
 * 用于与 navigator.platform / userAgent 交叉验证
 */

import type { CollectorResult, UserAgentDataInfo, UADataBrand } from '../types';

// 扩展 Navigator 类型以支持 userAgentData
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

/**
 * 采集 UserAgentData 信息
 * 包括基础信息和高熵值信息
 */
export const collectUserAgentData = async (): Promise<CollectorResult<UserAgentDataInfo>> => {
  const start = performance.now();

  try {
    const nav = window.navigator as ExtendedNavigator;

    // 检查 API 可用性 (Safari / Firefox 不支持)
    if (!nav.userAgentData) {
      return {
        status: 'unsupported',
        error: 'navigator.userAgentData not supported',
        timing: performance.now() - start
      };
    }

    const uaData = nav.userAgentData;

    // 基础信息 (同步获取)
    const baseData: UserAgentDataInfo = {
      brands: uaData.brands || [],
      mobile: uaData.mobile,
      platform: uaData.platform
    };

    // 尝试获取高熵值 (异步，可能被拒绝)
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
      // 高熵值获取失败，使用基础数据
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
