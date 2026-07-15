/**
 * 环境信息采集器 (Environment)
 * 浏览器/系统环境信息，易获取但也易伪造
 */

import { collectNavigator } from './navigator';
import { collectUserAgentData } from './userAgentData';
import { collectScreen } from './screen';
import { collectTimezone } from './timezone';
import type { EnvironmentResults } from '../types';

/**
 * 并行采集所有环境信息
 */
export const collectEnvironment = async (): Promise<EnvironmentResults> => {
  const [navigator, userAgentData, screen, timezone] = await Promise.all([
    collectNavigator(),
    collectUserAgentData(),
    collectScreen(),
    collectTimezone(),
  ]);

  return {
    navigator,
    userAgentData,
    screen,
    timezone,
  };
};

// 导出单个采集器
export { collectNavigator } from './navigator';
export { collectUserAgentData } from './userAgentData';
export { collectScreen } from './screen';
export { collectTimezone } from './timezone';
