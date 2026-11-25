import { isAutomatedCrawler } from './crawler';

export type ToolPage = 'home' | 'dns' | 'webrtc' | 'ipv6';
export type SpecialistTool = 'dns' | 'webrtc' | 'ipv6';

type UmamiProps = Record<string, string | number | boolean>;

declare global {
  interface Window {
    umami?: {
      track: (event: string, data?: UmamiProps) => void;
    };
  }
}

/** 上报无隐私信息的轻量交互事件；爬虫与服务端渲染下自动跳过。 */
export function trackEvent(event: string, data?: UmamiProps): void {
  if (typeof window === 'undefined' || isAutomatedCrawler()) return;
  try {
    window.umami?.track(event, data);
  } catch {
    /* ignore analytics errors */
  }
}

export function specialistToolFromHref(href?: string): SpecialistTool | null {
  if (!href) return null;
  if (href.startsWith('/dns-leak-test')) return 'dns';
  if (href.startsWith('/webrtc-leak-test')) return 'webrtc';
  if (href.startsWith('/ipv6-leak-test')) return 'ipv6';
  return null;
}
