import { getMessages, joinList, localePath, type Locale } from '../i18n';
import { dnsProvider } from './dnsPresentation';
import { canonicalIP, sameIP, sameIPv6Prefix64 } from './ip';
import { maskIP, maskIPsInText } from './ipDisplay';
import type { ReportModel, ReadoutRow } from './report';
import type { DNSExit, DNSVerdict } from './types';
import type { WebRTCVerdictReason, WebRTCVerdictState } from './webrtcCheck';
import type { IPv6VerdictReason, IPv6VerdictState, StackExit } from './ipv6Check';

const SITE_ORIGIN = 'https://detectradar.com';

export function maskIPText(ip: string): string {
  return maskIP(ip, 'star');
}

export function maskIPsInPlainText(text: string): string {
  return maskIPsInText(text, 'star');
}

function cleanPeriod(text: string): string {
  return text.trim().replace(/[。；.;]+$/, '');
}

function wrapParen(text: string, locale: Locale): string {
  return locale === 'en' ? ` (${text})` : `（${text}）`;
}

function kv(label: string, value: string, locale: Locale): string {
  return locale === 'en' ? `- ${label}: ${value}` : `- ${label}：${value}`;
}

function formatExitLine(
  ip: string | undefined,
  parts: (string | undefined)[],
  extraTags: string[] = [],
  locale: Locale = 'zh-CN',
): string {
  const masked = ip ? maskIPText(ip) : '';
  const meta = [...parts, ...extraTags].map((s) => s?.trim()).filter(Boolean);
  if (!masked && meta.length === 0) return getMessages(locale).common.unavailable;
  if (!masked) return meta.join(' · ');
  if (meta.length === 0) return masked;
  return `${masked}${wrapParen(meta.join(' · '), locale)}`;
}

function formatVerdictWithNote(title: string, note: string, locale: Locale): string {
  if (!note) return title;
  return locale === 'en' ? `${title} — ${note}` : `${title}${wrapParen(note, locale)}`;
}

function formatConsistencyRow(row: ReadoutRow, locale: Locale): string {
  const m = getMessages(locale);
  const s = m.home.shareText;
  if (row.state === 'coherent') return s.consistencyOk(row.label);
  if (row.compare && row.compare.ip !== m.common.unknown) {
    return s.consistencyMismatch(row.label, row.compare.browser, row.compare.ip);
  }
  return `${row.label}${locale === 'en' ? ': ' : '：'}${row.value}`;
}

function formatHomeDNSLeak(row: ReadoutRow, locale: Locale): string {
  const s = getMessages(locale).home.shareText;
  if (row.state === 'leak') {
    const names = row.meta?.dnsVerdict?.networks?.map((n) => dnsProvider(n)).filter(Boolean);
    if (names && names.length > 0) {
      return s.dnsLeakWithNames(joinList(names, locale));
    }
    return s.dnsLeakFallback;
  }
  return s.dnsStatusLine(row.value);
}

function formatHomeWebRTCLeak(row: ReadoutRow, locale: Locale): string {
  const s = getMessages(locale).home.shareText;
  if (row.state === 'leak') return s.webrtcLeak;
  if (row.state === 'coherent') return s.webrtcClear;
  return s.webrtcUnknown;
}

function formatHomeIPv6Leak(row: ReadoutRow, locale: Locale): string {
  const s = getMessages(locale).home.shareText;
  if (row.state === 'leak' || row.meta?.ipv6Kind === 'leaked') return s.ipv6Leak;
  if (row.meta?.ipv6Kind === 'unknown') return s.ipv6Unknown;
  if (row.meta?.ipv6Kind === 'dual-stack') return s.ipv6Dual;
  if (row.meta?.ipv6Kind === 'matches-exit') return s.ipv6Match;
  return s.ipv6None;
}

export function formatHomeShareText(report: ReportModel, locale: Locale = 'zh-CN'): string {
  const m = getMessages(locale);
  const s = m.home.shareText;
  const pipe = locale === 'en' ? ' | ' : '｜';
  const titleLine = report.scenario === 'daily' ? s.titleDaily : s.titleStrict;
  const lines: string[] = [titleLine];

  const abnormalCount = report.findings.filter(
    (f) => f.severity === 'leak' || f.severity === 'contradiction' || f.severity === 'caution',
  ).length;
  const summary =
    report.verdict.state === 'coherent' && abnormalCount === 0
      ? m.home.diagnosis.noIssues
      : abnormalCount > 0
        ? s.summaryWithCount(report.verdict.headline, abnormalCount)
        : report.verdict.headline;
  lines.push(kv(s.summaryLabel, summary, locale));

  if (report.identity.present) {
    const extraTags: string[] = [];
    if (report.identity.blacklist.length > 0) extraTags.push(s.blacklistTag);
    if (report.identity.openPorts.length > 0 && report.identity.exposure.some((t) => t.startsWith('open_proxy_port:'))) {
      extraTags.push(s.openProxyTag);
    }
    const geo = report.identity.geo && report.identity.geo !== '—' ? report.identity.geo : '';
    const asn = report.identity.asn && report.identity.asn !== '—' ? report.identity.asn : '';
    const usage = report.identity.status === 'ready' ? report.identity.usageLabel : '';
    lines.push(kv(s.exitLabel, formatExitLine(report.identity.ip, [geo, asn, usage], extraTags, locale), locale));
  } else {
    lines.push(kv(s.exitLabel, m.common.unavailable, locale));
  }

  if (report.consistency.length > 0) {
    const allCoherent = report.consistency.every((r) => r.state === 'coherent');
    lines.push(
      kv(
        s.consistencyLabel,
        allCoherent ? s.consistencyAllOk : report.consistency.map((r) => formatConsistencyRow(r, locale)).join(pipe),
        locale,
      ),
    );
  }

  if (report.leaks.length > 0) {
    const leakParts: string[] = [];
    const dnsRow = report.leaks.find((r) => r.key === 'dns');
    const webrtcRow = report.leaks.find((r) => r.key === 'webrtc');
    const ipv6Row = report.leaks.find((r) => r.key === 'ipv6');
    if (dnsRow) leakParts.push(formatHomeDNSLeak(dnsRow, locale));
    if (webrtcRow) leakParts.push(formatHomeWebRTCLeak(webrtcRow, locale));
    if (ipv6Row) leakParts.push(formatHomeIPv6Leak(ipv6Row, locale));
    if (leakParts.length > 0) {
      lines.push(kv(s.leaksLabel, leakParts.join(pipe), locale));
    }
  }

  const fpAbnormal = report.environment
    .filter((r) => r.key !== 'hardware' && r.state !== 'coherent')
    .map((r) => `${r.label}${locale === 'en' ? ': ' : ''}${r.value}`);
  lines.push(kv(s.fpLabel, fpAbnormal.length > 0 ? fpAbnormal.join(pipe) : m.home.diagnosis.noIssues, locale));

  lines.push(kv(s.retestLabel, `${SITE_ORIGIN}${localePath('/', locale)}`, locale));
  return maskIPsInPlainText(lines.join('\n'));
}

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

export function formatDNSShareText(data: DNSShareTextInput, locale: Locale = 'zh-CN'): string {
  const m = getMessages(locale);
  const s = m.dns.share;
  const lines: string[] = [s.textTitle];

  let reasonText = cleanPeriod(data.note);
  if (data.verdict?.reason === 'cross-region-isp' || data.verdict?.reason === 'mainland-carrier') {
    const names = data.verdict.networks?.map((n) => dnsProvider(n)).filter(Boolean) ?? [];
    if (names.length) reasonText = s.crossRegionShort(joinList(names, locale));
  } else if (data.verdict?.reason === 'proxy-isp-bypass') {
    const names = data.verdict.networks?.map((n) => dnsProvider(n)).filter(Boolean) ?? [];
    if (names.length) reasonText = s.proxyBypassShort(joinList(names, locale));
  }
  lines.push(kv(s.verdictLabel, formatVerdictWithNote(data.verdictTitle, reasonText, locale), locale));

  const exitParts = data.exit?.ip
    ? [
        data.exitRegion,
        data.exit.network ? data.exitNetworkName : '',
        data.exit.asn,
        data.exitTypeLabel,
      ]
    : [];
  lines.push(kv(s.exitLabel, formatExitLine(data.exit?.ip, exitParts, [], locale), locale));

  if (data.providers.length > 0) {
    const sep = locale === 'en' ? ', ' : '，';
    const providerText = joinList(
      data.providers.map((p) => {
        const typeTag =
          p.type === 'public'
            ? m.dns.page.tagPublic
            : p.type === 'access'
              ? m.dns.page.tagAccess
              : '';
        const meta = [p.regions, typeTag].filter(Boolean).join(sep);
        return meta ? `${p.name}${wrapParen(meta, locale)}` : p.name;
      }),
      locale,
    );
    lines.push(kv(s.dnsLabel, providerText, locale));
  } else {
    lines.push(kv(s.dnsLabel, s.dnsNone, locale));
  }

  if (data.comparisonSummary) {
    lines.push(kv(s.changeLabel, cleanPeriod(data.comparisonSummary), locale));
  }

  lines.push(kv(s.retestLabel, `${SITE_ORIGIN}${localePath('/dns-leak-test/', locale)}`, locale));
  return maskIPsInPlainText(lines.join('\n'));
}

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

export function formatWebRTCShareText(data: WebRTCShareTextInput, locale: Locale = 'zh-CN'): string {
  const m = getMessages(locale);
  const p = m.webrtc.page;
  const s = m.webrtc.share;
  const pipe = locale === 'en' ? ' | ' : '｜';
  const lines: string[] = [s.textTitle];

  const cleanNote = cleanPeriod(data.verdictNote);
  lines.push(kv(s.verdictLabel, formatVerdictWithNote(data.verdictTitle, cleanNote, locale), locale));

  const exitParts = data.exit?.ip
    ? [
        data.exitRegion,
        data.exit.network ? data.exitNetworkName : '',
        data.exit.asn,
        data.exitTypeLabel,
      ]
    : [];
  lines.push(kv(s.exitLabel, formatExitLine(data.exit?.ip, exitParts, [], locale), locale));

  if (!data.apiSupported) {
    lines.push(kv(s.publicLabel, s.publicDisabled, locale));
  } else if (data.publicIPs.length === 0) {
    lines.push(kv(s.publicLabel, s.publicNone, locale));
  } else {
    const suspectSet = new Set(data.suspectIPs.map((ip) => canonicalIP(ip)));
    const exitIP = data.exit?.ip ?? '';
    const pubText = joinList(
      data.publicIPs.map((ip) => {
        const canon = canonicalIP(ip);
        const masked = maskIPText(canon);
        if (suspectSet.has(canon)) return `${masked}${wrapParen(p.tagDifferent, locale)}`;
        if (sameIP(canon, exitIP)) return `${masked}${wrapParen(p.tagMatch, locale)}`;
        if (sameIPv6Prefix64(canon, exitIP)) return `${masked}${wrapParen(p.tagSamePrefix, locale)}`;
        if (data.verdictReason === 'dual-stack') return `${masked}${wrapParen(p.tagSameISP, locale)}`;
        if (data.verdictReason === 'same-network') return `${masked}${wrapParen(p.tagSharedExit, locale)}`;
        return masked;
      }),
      locale,
    );
    lines.push(kv(s.publicLabel, pubText, locale));
  }

  const localText = !data.apiSupported
    ? p.apiDisabled
    : data.localIPs.length > 0
      ? joinList(data.localIPs.map((item) => `${maskIPText(item.ip)}${wrapParen(item.scope, locale)}`), locale)
      : data.mdnsCount > 0
        ? p.mdnsHidden
        : s.localNone;
  const ipv6Text = !data.apiSupported
    ? s.ipv6SkippedText
    : data.ipv6Global
      ? s.ipv6Found(maskIPText(data.ipv6Global))
      : p.ipv6None;
  lines.push(kv(s.localAndV6Label, `${localText}${pipe}${ipv6Text}`, locale));

  if (data.comparisonSummary) {
    lines.push(kv(s.changeLabel, cleanPeriod(data.comparisonSummary), locale));
  }

  lines.push(kv(s.retestLabel, `${SITE_ORIGIN}${localePath('/webrtc-leak-test/', locale)}`, locale));
  return maskIPsInPlainText(lines.join('\n'));
}

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

export function formatIPv6ShareText(data: IPv6ShareTextInput, locale: Locale = 'zh-CN'): string {
  const m = getMessages(locale);
  const p = m.ipv6.page;
  const s = m.ipv6.share;
  const lines: string[] = [s.textTitle];

  const cleanNote = cleanPeriod(data.verdictNote);
  lines.push(kv(s.verdictLabel, formatVerdictWithNote(data.verdictTitle, cleanNote, locale), locale));

  const v4Parts = data.v4?.ip
    ? [
        data.v4Region,
        data.v4.network ? data.v4NetworkName : '',
        data.v4.asn,
        data.v4TypeLabel,
      ]
    : [];
  lines.push(kv(s.v4Label, data.v4?.ip ? formatExitLine(data.v4.ip, v4Parts, [], locale) : p.unreachableShort, locale));

  if (data.v6?.ip) {
    const v6Parts = [
      data.v6Region,
      data.v6.network ? data.v6NetworkName : '',
      data.v6.asn,
      data.v6TypeLabel,
    ];
    lines.push(kv(s.v6Label, formatExitLine(data.v6.ip, v6Parts, [], locale), locale));
  } else {
    lines.push(kv(s.v6Label, data.v4?.ip ? p.v6Unreachable : p.unreachableShort, locale));
  }

  if (data.comparisonSummary) {
    lines.push(kv(s.changeLabel, cleanPeriod(data.comparisonSummary), locale));
  }

  lines.push(kv(s.retestLabel, `${SITE_ORIGIN}${localePath('/ipv6-leak-test/', locale)}`, locale));
  return maskIPsInPlainText(lines.join('\n'));
}

export async function copyShareText(text: string, locale: Locale = 'zh-CN'): Promise<string> {
  const c = getMessages(locale).common;
  try {
    await navigator.clipboard.writeText(text);
    return c.textCopied;
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
      if (copied) return c.textCopied;
    } catch {
    }
    return c.copyFailed;
  }
}
