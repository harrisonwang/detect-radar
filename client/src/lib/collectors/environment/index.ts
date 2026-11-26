
import { collectNavigator } from './navigator';
import { collectUserAgentData } from './userAgentData';
import { collectScreen } from './screen';
import { collectTimezone } from './timezone';
import type { EnvironmentResults } from '../types';

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

export { collectNavigator } from './navigator';
export { collectUserAgentData } from './userAgentData';
export { collectScreen } from './screen';
export { collectTimezone } from './timezone';
