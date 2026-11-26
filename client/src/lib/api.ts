
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

async function request<T>(
  path: string,
  init: RequestInit = {},
  timeoutMs = 15_000,
): Promise<T> {
  const url = `${API_BASE}${path}`;
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
      throw new ApiError(0, `请求超时 (>${timeoutMs}ms)`, url);
    }
    throw new ApiError(0, (err as Error)?.message ?? 'network error', url);
  } finally {
    clearTimeout(timer);
  }
}

export const createScan = (req: ScanRequest) =>
  request<ScanResponse>('/scans', { method: 'POST', body: JSON.stringify(req) }, 20_000);

export const getScan = (id: string, scenario?: ScanScenario) => {
  const qs = scenario ? `?scenario=${encodeURIComponent(scenario)}` : '';
  return request<ScanResponse>(`/scans/${encodeURIComponent(id)}${qs}`);
};

export const updateScanDNS = (id: string, leaked: boolean) =>
  request<ScanResponse>(`/scans/${encodeURIComponent(id)}/dns`, {
    method: 'POST',
    body: JSON.stringify({ leaked }),
  });

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
) =>
  request<void>(`/scans/${encodeURIComponent(id)}/feedback`, {
    method: 'POST',
    body: JSON.stringify({ category, note, reply_email: replyEmail }),
  });

export const createDNSLeakTest = (scanId?: string) =>
  request<DNSLeakTestResponse>('/leaks/dns', {
    method: 'POST',
    body: JSON.stringify(scanId ? { scan_id: scanId } : {}),
  });

export const getDNSLeakResult = (id: string) =>
  request<DNSLeakResult>(`/leaks/dns/${encodeURIComponent(id)}`, { cache: 'no-store' });

export const getBrowserExit = () => request<IPIntel>('/ip', { cache: 'no-store' });

export const assessDNSObservation = (observation: DNSAssessmentInput) =>
  request<DNSAssessmentResult>('/leaks/dns/assessment', { method: 'POST', body: JSON.stringify(observation) }, 6000);

export const assessWebRTCCandidates = (publicIPs: string[]) =>
  request<WebRTCAssessmentResponse>(
    '/leaks/webrtc/assessment',
    { method: 'POST', body: JSON.stringify({ public_ips: publicIPs }) },
    6000,
  );
