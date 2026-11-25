/**
 * 分享文字结果生成器：把首页、DNS 页和 WebRTC 页的检测结果整理成
 * 适合直接粘贴到论坛、群聊或 GitHub Issue 的简洁文本，并自动隐藏完整 IP。
 */

import { canonicalIP, sameIP, sameIPv6Prefix64 } from './ip';
import type { ReportModel, ReadoutRow } from './report';
import type { DNSExit, DNSVerdict } from './types';
import type { WebRTCVerdictReason, WebRTCVerdictState } from './webrtcCheck';
import type { IPv6VerdictReason, IPv6VerdictState, StackExit } from './ipv6Check';

const SITE_ORIGIN = 'https://detectradar.com';

const IPV4_TEXT_RE = /\b(\d{1,3}\.\d{1,3})\.\d{1,3}\.\d{1,3}\b/g;
// 匹配标准 3 段以上 IPv6 以及包含 "::" 压缩的 IPv6（避开 10:30:05 这类时间字符串：
// 至少包含一个字母或双冒号，或满 4 段以上十六进制）。
const IPV6_COMPRESSED_RE = /\b([0-9a-fA-F]{1,4}(?::[0-9a-fA-F]{1,4})?)::[0-9a-fA-F:]*\b/g;
const IPV6_FULL_RE = /\b([0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}):(?:[0-9a-fA-F]{1,4}:){2,}[0-9a-fA-F]{1,4}\b/g;

/** 文本场景使用的 IP 脱敏：IPv4 → 203.0.*.*，IPv6 → 2001:db8:* */
export function maskIPText(ip: string): string {
  const raw = ip.trim();
  if (!raw) return '—';
  const v4 = /^(\d{1,3}\.\d{1,3})\.\d{1,3}\.\d{1,3}$/.exec(raw);
  if (v4) return `${v4[1]}.*.*`;
  if (raw.includes(':')) {
    const parts = raw.split(':').filter((p) => p.length > 0);
    if (parts.length >= 2) return `${parts[0]}:${parts[1]}:*`;
    if (parts.length === 1) return `${parts[0]}:*`;
    return '*:*';
  }
  return '***';
}

/** 对任意自然语言文本中意外携带的完整 IPv4 / IPv6 做兜底脱敏 */
export function maskIPsInPlainText(text: string): string {
  return text
    .replace(IPV4_TEXT_RE, '$1.*.*')
    .replace(IPV6_FULL_RE, '$1:*')
    .replace(IPV6_COMPRESSED_RE, '$1:*');
}

function cleanPeriod(text: string): string {
  return text.trim().replace(/[。；;]+$/, '');
}

function formatExitLine(
  ip: string | undefined,
  parts: (string | undefined)[],
  extraTags: string[] = [],
): string {
  const masked = ip ? maskIPText(ip) : '';
  const meta = [...parts, ...extraTags].map((s) => s?.trim()).filter(Boolean);
  if (!masked && meta.length === 0) return '未获取';
  if (!masked) return meta.join(' · ');
  if (meta.length === 0) return masked;
  return `${masked}（${meta.join(' · ')}）`;
}

// ============================================================================
// 首页综合报告文字结果
// ============================================================================

function formatConsistencyRow(row: ReadoutRow): string {
  if (row.state === 'coherent') return `${row.label}一致`;
  if (row.compare) {
    return `${row.label}不一致（浏览器 ${row.compare.browser} / 出口 ${row.compare.ip}）`;
  }
  return `${row.label}${row.value}`;
}

function formatHomeDNSLeak(row: ReadoutRow): string {
  if (row.value === '泄露') {
    // 从 dnsVerdictNote("出口 IP 在海外，测到了中国移动的 DNS。") 中提取更顺口的日常表述
    const m = row.hint?.match(/测到了(.+?)的 DNS/);
    if (m && m[1]) {
      return `DNS 泄露（检出${m[1]} DNS）`;
    }
    return 'DNS 泄露（检出国内运营商 DNS）';
  }
  return `DNS ${row.value}`;
}

function formatHomeWebRTCLeak(row: ReadoutRow): string {
  if (row.state === 'leak') return 'WebRTC 泄露';
  if (row.state === 'coherent') return 'WebRTC 未泄露';
  return 'WebRTC 无法判断';
}

function formatHomeIPv6Leak(row: ReadoutRow): string {
  if (row.state === 'leak') return 'IPv6 泄露';
  if (row.value === '检测到 IPv6 双栈') return 'IPv6 双栈正常';
  if (row.value === '与出口 IP 一致') return 'IPv6 与出口一致';
  return '未测到 IPv6';
}

export function formatHomeShareText(report: ReportModel): string {
  const titleLine =
    report.scenario === 'daily'
      ? 'DetectRadar 网络环境检测结果（日常模式）'
      : 'DetectRadar 网络环境检测结果';
  const lines: string[] = [titleLine];

  // 1. 综合结论（无异常时统一使用「未见明显异常」）
  const abnormalCount = report.findings.filter(
    (f) => f.severity === 'leak' || f.severity === 'contradiction' || f.severity === 'caution',
  ).length;
  const summary =
    report.verdict.state === 'coherent' && abnormalCount === 0
      ? '未见明显异常'
      : abnormalCount > 0
        ? `${report.verdict.headline}（发现 ${abnormalCount} 项异常）`
        : report.verdict.headline;
  lines.push(`- 综合结论：${summary}`);

  // 2. 出口信息
  if (report.identity.present) {
    const extraTags: string[] = [];
    if (report.identity.blacklist.length > 0) extraTags.push('命中黑名单');
    if (report.identity.openPorts.length > 0 && report.identity.exposure.some((t) => t.startsWith('open_proxy_port:'))) {
      extraTags.push('开放代理端口');
    }
    const geo = report.identity.geo && report.identity.geo !== '—' ? report.identity.geo : '';
    const asn = report.identity.asn && report.identity.asn !== '—' ? report.identity.asn : '';
    const usage = report.identity.usageLabel && report.identity.usageLabel !== '检测中' ? report.identity.usageLabel : '';
    lines.push(`- 出口信息：${formatExitLine(report.identity.ip, [geo, asn, usage], extraTags)}`);
  } else {
    lines.push('- 出口信息：未获取');
  }

  // 3. 环境一致性
  if (report.consistency.length > 0) {
    const allCoherent = report.consistency.every((r) => r.state === 'coherent');
    lines.push(
      `- 环境核对：${allCoherent ? '时区、语言均与出口一致' : report.consistency.map(formatConsistencyRow).join('｜')}`,
    );
  }

  // 4. 泄露检查
  if (report.leaks.length > 0) {
    const leakParts: string[] = [];
    const dnsRow = report.leaks.find((r) => r.label === 'DNS');
    const webrtcRow = report.leaks.find((r) => r.label === 'WebRTC');
    const ipv6Row = report.leaks.find((r) => r.label === 'IPv6');
    if (dnsRow) leakParts.push(formatHomeDNSLeak(dnsRow));
    if (webrtcRow) leakParts.push(formatHomeWebRTCLeak(webrtcRow));
    if (ipv6Row) leakParts.push(formatHomeIPv6Leak(ipv6Row));
    if (leakParts.length > 0) {
      lines.push(`- 泄露检查：${leakParts.join('｜')}`);
    }
  }

  // 5. 环境指纹（硬件参数仅展示不计分，汇总时只呈现异常项或“未见明显异常”）
  const fpAbnormal = report.environment
    .filter((r) => r.label !== '硬件' && r.state !== 'coherent')
    .map((r) => `${r.label}${r.value}`);
  lines.push(`- 环境指纹：${fpAbnormal.length > 0 ? fpAbnormal.join('｜') : '未见明显异常'}`);

  lines.push(`- 在线复测：${SITE_ORIGIN}/`);
  return maskIPsInPlainText(lines.join('\n'));
}

// ============================================================================
// DNS 泄露专项页文字结果
// ============================================================================

export interface DNSShareTextInput {
  verdictState: 'suspected' | 'clear' | 'unknown' | 'unobserved';
  verdictTitle: string;
  verdict?: DNSVerdict;
  note: string;
  exit: DNSExit | null;
  exitNetworkName: string;
  exitRegion: string;
  exitTypeLabel: string;
  providers: {
    name: string;
    regions: string;
    type: 'public' | 'access' | 'hosting';
    suspect: boolean;
  }[];
  comparisonSummary?: string | null;
}

export function formatDNSShareText(data: DNSShareTextInput): string {
  const lines: string[] = ['DetectRadar DNS 泄露检测结果'];

  let reasonText = cleanPeriod(data.note);
  if (data.verdict?.reason === 'mainland-carrier') {
    const carriers = data.verdict.networks?.filter(Boolean).join('、') || '国内运营商';
    reasonText = `出口 IP 在海外，检出${carriers} DNS`;
  }
  lines.push(`- 检测结论：${data.verdictTitle}${reasonText ? `（${reasonText}）` : ''}`);

  const exitParts = data.exit?.ip
    ? [
        data.exitRegion,
        data.exitNetworkName !== '网络未知' && data.exitNetworkName !== '未获取' ? data.exitNetworkName : '',
        data.exit.asn,
        data.exitTypeLabel,
      ]
    : [];
  lines.push(`- 出口信息：${formatExitLine(data.exit?.ip, exitParts)}`);

  if (data.providers.length > 0) {
    const providerText = data.providers
      .map((p) => {
        const typeTag = p.type === 'public' ? '公共 DNS' : p.type === 'access' ? '运营商 DNS' : '';
        const meta = [p.regions, typeTag].filter(Boolean).join('，');
        return meta ? `${p.name}（${meta}）` : p.name;
      })
      .join('、');
    lines.push(`- 测得 DNS：${providerText}`);
  } else {
    lines.push('- 测得 DNS：未收到解析记录');
  }

  if (data.comparisonSummary) {
    lines.push(`- 复测变化：${cleanPeriod(data.comparisonSummary)}`);
  }

  lines.push(`- 在线复测：${SITE_ORIGIN}/dns-leak-test/`);
  return maskIPsInPlainText(lines.join('\n'));
}

// ============================================================================
// WebRTC 泄露专项页文字结果
// ============================================================================

export interface WebRTCShareTextInput {
  verdictState: WebRTCVerdictState;
  verdictTitle: string;
  verdictReason: WebRTCVerdictReason;
  verdictNote: string;
  exit: DNSExit | null;
  exitNetworkName: string;
  exitRegion: string;
  exitTypeLabel: string;
  apiSupported: boolean;
  publicIPs: string[];
  suspectIPs: string[];
  localIPs: { ip: string; scope: string }[];
  mdnsCount: number;
  ipv6Global: string | null;
  comparisonSummary?: string | null;
}

export function formatWebRTCShareText(data: WebRTCShareTextInput): string {
  const lines: string[] = ['DetectRadar WebRTC 泄露检测结果'];

  const cleanNote = cleanPeriod(data.verdictNote);
  lines.push(`- 检测结论：${data.verdictTitle}${cleanNote ? `（${cleanNote}）` : ''}`);

  const exitParts = data.exit?.ip
    ? [
        data.exitRegion,
        data.exitNetworkName !== '网络未知' && data.exitNetworkName !== '未获取' ? data.exitNetworkName : '',
        data.exit.asn,
        data.exitTypeLabel,
      ]
    : [];
  lines.push(`- 出口信息：${formatExitLine(data.exit?.ip, exitParts)}`);

  if (!data.apiSupported) {
    lines.push('- 公网候选：浏览器已禁用 WebRTC');
  } else if (data.publicIPs.length === 0) {
    lines.push('- 公网候选：未暴露公网地址');
  } else {
    const suspectSet = new Set(data.suspectIPs.map((ip) => canonicalIP(ip)));
    const exitIP = data.exit?.ip ?? '';
    const pubText = data.publicIPs
      .map((ip) => {
        const canon = canonicalIP(ip);
        const masked = maskIPText(canon);
        if (suspectSet.has(canon)) return `${masked}（与出口不同）`;
        if (sameIP(canon, exitIP)) return `${masked}（与出口一致）`;
        if (sameIPv6Prefix64(canon, exitIP)) return `${masked}（同网段 IPv6）`;
        if (data.verdictReason === 'dual-stack') return `${masked}（同运营商）`;
        if (data.verdictReason === 'same-network') return `${masked}（共享出口）`;
        return masked;
      })
      .join('、');
    lines.push(`- 公网候选：${pubText}`);
  }

  const localText = !data.apiSupported
    ? '接口已禁用'
    : data.localIPs.length > 0
      ? data.localIPs.map((item) => `${maskIPText(item.ip)}（${item.scope}）`).join('、')
      : data.mdnsCount > 0
        ? 'mDNS 已隐藏（*.local）'
        : '未暴露内网地址';
  const ipv6Text = !data.apiSupported
    ? '禁用 WebRTC 时未采集 ICE IPv6'
    : data.ipv6Global
      ? `测得全局 IPv6（${maskIPText(data.ipv6Global)}）`
      : '未测到全局 IPv6';
  lines.push(`- 内网与 IPv6：${localText}｜${ipv6Text}`);

  if (data.comparisonSummary) {
    lines.push(`- 复测变化：${cleanPeriod(data.comparisonSummary)}`);
  }

  lines.push(`- 在线复测：${SITE_ORIGIN}/webrtc-leak-test/`);
  return maskIPsInPlainText(lines.join('\n'));
}

// ============================================================================
// IPv6 泄露专项页文字结果
// ============================================================================

export interface IPv6ShareTextInput {
  verdictState: IPv6VerdictState;
  verdictTitle: string;
  verdictReason: IPv6VerdictReason;
  verdictNote: string;
  v4: StackExit | null;
  v4NetworkName: string;
  v4Region: string;
  v4TypeLabel: string;
  v6: StackExit | null;
  v6NetworkName: string;
  v6Region: string;
  v6TypeLabel: string;
  comparisonSummary?: string | null;
}

export function formatIPv6ShareText(data: IPv6ShareTextInput): string {
  const lines: string[] = ['DetectRadar IPv6 泄露检测结果'];

  const cleanNote = cleanPeriod(data.verdictNote);
  lines.push(`- 检测结论：${data.verdictTitle}${cleanNote ? `（${cleanNote}）` : ''}`);

  const v4Parts = data.v4?.ip
    ? [
        data.v4Region,
        data.v4NetworkName !== '网络未知' && data.v4NetworkName !== '未连通' ? data.v4NetworkName : '',
        data.v4.asn,
        data.v4TypeLabel,
      ]
    : [];
  lines.push(`- IPv4 出口：${data.v4?.ip ? formatExitLine(data.v4.ip, v4Parts) : '未连通'}`);

  if (data.v6?.ip) {
    const v6Parts = [
      data.v6Region,
      data.v6NetworkName !== '网络未知' && data.v6NetworkName !== '未连通' ? data.v6NetworkName : '',
      data.v6.asn,
      data.v6TypeLabel,
    ];
    lines.push(`- IPv6 出口：${formatExitLine(data.v6.ip, v6Parts)}`);
  } else {
    lines.push('- IPv6 出口：未连通（仅 IPv4 网络）');
  }

  if (data.comparisonSummary) {
    lines.push(`- 复测变化：${cleanPeriod(data.comparisonSummary)}`);
  }

  lines.push(`- 在线复测：${SITE_ORIGIN}/ipv6-leak-test/`);
  return maskIPsInPlainText(lines.join('\n'));
}

// ============================================================================
// 剪贴板写入（优先 Clipboard API，降级 execCommand）
// ============================================================================

export async function copyShareText(text: string): Promise<string> {
  try {
    await navigator.clipboard.writeText(text);
    return '已复制文字';
  } catch {
    try {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.setAttribute('readonly', '');
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      textarea.style.pointerEvents = 'none';
      document.body.appendChild(textarea);
      textarea.select();
      const copied = document.execCommand('copy');
      textarea.remove();
      if (copied) return '已复制文字';
    } catch {
      /* 浏览器禁用剪贴板写入 */
    }
    return '复制失败，请检查浏览器剪贴板权限';
  }
}
