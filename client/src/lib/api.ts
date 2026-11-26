import { apiLang, detectClientLocale, type Locale } from '../i18n';
import type {
  ScanRequest,
  ScanResponse,
  ScanScenario,
  DNSLeakTestResponse,
  DNSLeakResult,
  DNSAssessmentInput,
  DNSAssessmentResult,
  WebRTCAssessmentResponse,
  IPIntel,
} from './types';

const ENV = (import.meta as unknown as { env?: Record<string, string | boolean> }).env ?? {};
const RAW_BASE =
  typeof ENV.PUBLIC_API_BASE === 'string'
    ? ENV.PUBLIC_API_BASE
    : ENV.DEV
      ? 'http://127.0.0.1:8080'
      : '';

export const API_BASE = RAW_BASE ? `${RAW_BASE.replace(/\/+$/, '')}/api/v1` : '/api/v1';

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: string,
    url: string,
  ) {
    super(`API ${status} @ ${url}: ${body.slice(0, 200)}`);
    this.name = 'ApiError';
  }
}

function appendLang(path: string, locale?: Locale): string {
  const lang = apiLang(locale ?? detectClientLocale());
  const sep = path.includes('?') ? '&' : '?';
  return `${path}${sep}lang=${lang}`;
}

async function request<T>(
  path: string,
  init: RequestInit = {},
  timeoutMs = 15_000,
  locale?: Locale,
): Promise<T> {
  const effectiveLocale = locale ?? detectClientLocale();
  const url = `${API_BASE}${appendLang(path, effectiveLocale)}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const res = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        ...(init.body ? { 'Content-Type': 'application/json' } : {}),
        ...init.headers,
      },
    });

    const text = await res.text();
    if (!res.ok) {
      throw new ApiError(res.status, text, url);
    }
    return (text ? JSON.parse(text) : {}) as T;
  } catch (err) {
    if (err instanceof ApiError) throw err;
    if ((err as Error)?.name === 'AbortError') {
      const msg =
        effectiveLocale === 'en'
          ? `Request timed out (>${timeoutMs}ms)`
          : `请求超时 (>${timeoutMs}ms)`;
      throw new ApiError(0, msg, url);
    }
    throw new ApiError(0, (err as Error)?.message ?? 'network error', url);
  } finally {
    clearTimeout(timer);
  }
}

export const createScan = (req: ScanRequest, locale?: Locale) =>
  request<ScanResponse>('/scans', { method: 'POST', body: JSON.stringify(req) }, 20_000, locale);

export const getScan = (id: string, scenario?: ScanScenario, locale?: Locale) => {
  const qs = scenario ? `?scenario=${encodeURIComponent(scenario)}` : '';
  return request<ScanResponse>(`/scans/${encodeURIComponent(id)}${qs}`, {}, 15_000, locale);
};

export type FeedbackCategory =
  | 'false_positive'
  | 'missed_detection'
  | 'data_wrong'
  | 'other';

export const submitScanFeedback = (
  id: string,
  category: FeedbackCategory,
  note?: string,
  replyEmail?: string,
  locale?: Locale,
) =>
  request<void>(
    `/scans/${encodeURIComponent(id)}/feedback`,
    {
      method: 'POST',
      body: JSON.stringify({ category, note, reply_email: replyEmail }),
    },
    15_000,
    locale,
  );

export const createDNSLeakTest = (scanId?: string, locale?: Locale) =>
  request<DNSLeakTestResponse>(
    '/leaks/dns',
    {
      method: 'POST',
      body: JSON.stringify(scanId ? { scan_id: scanId } : {}),
    },
    15_000,
    locale,
  );

export const getDNSLeakResult = (id: string, locale?: Locale) =>
  request<DNSLeakResult>(`/leaks/dns/${encodeURIComponent(id)}`, { cache: 'no-store' }, 15_000, locale);

export const getBrowserExit = (locale?: Locale) =>
  request<IPIntel>('/ip', { cache: 'no-store' }, 15_000, locale);

export const assessDNSObservation = (observation: DNSAssessmentInput, locale?: Locale) =>
  request<DNSAssessmentResult>(
    '/leaks/dns/assessment',
    { method: 'POST', body: JSON.stringify(observation) },
    6000,
    locale,
  );

export const assessWebRTCCandidates = (publicIPs: string[], locale?: Locale) =>
  request<WebRTCAssessmentResponse>(
    '/leaks/webrtc/assessment',
    { method: 'POST', body: JSON.stringify({ public_ips: publicIPs }) },
    6000,
    locale,
  );
