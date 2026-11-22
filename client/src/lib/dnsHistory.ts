import type { DNSCheck } from './dnsCheck';
import { emptyHistory, recentHistory, type RecentHistory } from './recentHistory';

export type DNSHistory = RecentHistory<DNSCheck>;

export const emptyDNSHistory: DNSHistory = emptyHistory<DNSCheck>();

export const dnsHistory = (next?: DNSCheck, lastInPage?: DNSCheck | null): Promise<DNSHistory> =>
  recentHistory<DNSCheck>('detectradar-dns', next, lastInPage);
