
import { collectNavigator } from './navigator';
import { collectUserAgentData } from './userAgentData';
import { collectTimezone } from './timezone';
import type { EnvironmentResults } from '../types';

export const collectEnvironment = async (): Promise<EnvironmentResults> => {
  const [navigator, userAgentData, timezone] = await Promise.all([
    collectNavigator(),
    collectUserAgentData(),
    collectTimezone(),
  ]);

  return {
    navigator,
    userAgentData,
    timezone,
  };
};

export { collectNavigator } from './navigator';
export { collectUserAgentData } from './userAgentData';
export { collectTimezone } from './timezone';
