/**
 * DetectRadar 后端 API 客户端
 *
 * 单一出入口：所有对 server (Go/Fiber) 的调用都走这里，集中处理
 * base URL、超时、JSON 解析与错误。base URL 通过环境变量注入，
 * 默认指向本地后端；生产部署时设置 PUBLIC_API_BASE 覆盖。
 *
 * 只覆盖前端实际调用的接口（扫描 IP 恒取自连接本身，不接受调用方指定任意 IP）：
 *   POST /scans                  GET  /scans/:id
 *   POST /scans/:id/dns          POST /scans/:id/feedback
 *   POST /leaks/dns              GET  /leaks/dns/:id
 * RTT 探针 GET /ping 由 collectors/network/rtt.ts 直接 fetch（需自行掐表计时，不走本层）。
 * 后端另有 GET /ip、GET /ip/reputation 两个公开自查端点，本 SPA 全程走 /scans，故不封装。
 */

import type {
  ScanRequest,
  ScanResponse,
  DNSLeakTestResponse,
  DNSLeakResult,
} from './types';

// Astro/Vite：仅 PUBLIC_ 前缀的变量会暴露到客户端
const RAW_BASE =
  (import.meta as unknown as { env?: Record<string, string> }).env?.PUBLIC_API_BASE ??
  'http://127.0.0.1:8080';

/** 规范化后的 API 根地址，如 http://127.0.0.1:8080/api/v1 */
export const API_BASE = `${RAW_BASE.replace(/\/+$/, '')}/api/v1`;

/** 后端返回非 2xx 时抛出，携带状态码与原始报文 */
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
    // 后端所有成功响应均为 JSON
    return (text ? JSON.parse(text) : {}) as T;
  } catch (err) {
    if (err instanceof ApiError) throw err;
    if ((err as Error)?.name === 'AbortError') {
      throw new ApiError(0, `请求超时 (>${timeoutMs}ms)`, url);
    }
    // 网络层错误（后端未启动 / CORS / DNS）
    throw new ApiError(0, (err as Error)?.message ?? 'network error', url);
  } finally {
    clearTimeout(timer);
  }
}

// ============================================================================
// 统一扫描（核心）
// ============================================================================

/** 提交采集信号，返回服务端聚合分析 + 评分 + 诊断 */
export const createScan = (req: ScanRequest) =>
  request<ScanResponse>('/scans', { method: 'POST', body: JSON.stringify(req) }, 20_000);

/** 按 scan_id 取回历史扫描结果（30 分钟内有效） */
export const getScan = (id: string) =>
  request<ScanResponse>(`/scans/${encodeURIComponent(id)}`);

/** 回传异步 DNS 泄露结果，后端更新该扫描并重新评分（评分始终纯后端产出） */
export const updateScanDNS = (id: string, leaked: boolean) =>
  request<ScanResponse>(`/scans/${encodeURIComponent(id)}/dns`, {
    method: 'POST',
    body: JSON.stringify({ leaked }),
  });

/** 结果反馈的四类语义（与后端 service.ValidFeedbackCategory 枚举一一对应） */
export type FeedbackCategory =
  | 'false_positive' // 我是正常网络，被误判
  | 'missed_detection' // 在用代理/VPN，但没检出
  | 'data_wrong' // 位置/运营商/ASN 等数据不对
  | 'other';

/**
 * 提交对某次扫描结论的反馈（误报/漏检/数据不对）。误报标注只能来自真实用户，
 * 是校准规则的唯一现场信号。后端仅落遥测流水，成功返回 204 无内容。
 */
export const submitScanFeedback = (
  id: string,
  category: FeedbackCategory,
  note?: string,
) =>
  request<void>(`/scans/${encodeURIComponent(id)}/feedback`, {
    method: 'POST',
    body: JSON.stringify(note ? { category, note } : { category }),
  });

// ============================================================================
// 独立泄露 / 一致性端点（/scans 已内含，这些用于单项复测）
// ============================================================================

/** 创建 DNS 泄露测试，返回一次性测试域名供客户端触发解析（scan_id 供服务端把结果关联回扫描） */
export const createDNSLeakTest = (scanId?: string) =>
  request<DNSLeakTestResponse>('/leaks/dns', {
    method: 'POST',
    body: JSON.stringify(scanId ? { scan_id: scanId } : {}),
  });

/** 拉取 DNS 泄露测试结果（客户端触发解析后轮询） */
export const getDNSLeakResult = (id: string) =>
  request<DNSLeakResult>(`/leaks/dns/${encodeURIComponent(id)}`);
