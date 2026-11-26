import { detectClientLocale } from '../i18n';
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

export function trackEvent(event: string, data?: UmamiProps): void {
  if (typeof window === 'undefined' || isAutomatedCrawler()) return;
  try {
    const payload: UmamiProps = {
      locale: detectClientLocale(),
      ...data,
    };
    window.umami?.track(event, payload);
  } catch {
  }
}

export function specialistToolFromHref(href?: string): SpecialistTool | null {
  if (!href) return null;
  const normalized = href.replace(/^\/en(?=\/|$)/, '') || '/';
  if (normalized.startsWith('/dns-leak-test')) return 'dns';
  if (normalized.startsWith('/webrtc-leak-test')) return 'webrtc';
  if (normalized.startsWith('/ipv6-leak-test')) return 'ipv6';
  return null;
}
