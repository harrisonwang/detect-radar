import {
  countryName,
  formatTimezoneZone,
  getMessages,
  joinList,
  languageName,
  localePath,
  type Locale,
} from "../i18n";
import type { AllCollectorResults } from "./collectors/types";
import type {
  ScanResponse,
  ScanScenario,
  ScanAnalysis,
  IdentityAnalysis,
  DNSExit,
  DNSVerdict,
  IPIntel,
  RiskLevel,
  DimensionScore,
  ConnectionCharacteristic,
} from "./types";

import { browserExit, DNS_POLICY_VERSION, dnsProvider, dnsVerdictNote, getDnsStatus } from "./dnsPresentation";
import { isIPv6Address, sameIP } from "./ip";
import { maskIP } from "./ipDisplay";

export type SignalState = "coherent" | "contradiction" | "leak" | "unknown";
export type Severity = "leak" | "contradiction" | "caution" | "info";

export interface Finding {
  id: string;
  severity: Severity;
  title: string;
  fact: string;
  fix: string;
}

export interface ReportGroup {
  key: "identity" | "consistency" | "leak" | "environment";
  label: string;
  score: number;
  state: SignalState;
  passed: number;
  total: number;
}

export interface ReadoutRow {
  key: string;
  label: string;
  value: string;
  state: SignalState;
  href?: string;
  hint?: string;
  compare?: { browser: string; ip: string };
  meta?: {
    dnsVerdict?: DNSVerdict;
    ipv6Kind?: "leaked" | "none" | "matches-exit" | "dual-stack" | "unknown";
  };
}

export interface IdentityReadout {
  ip: string;
  masked: string;
  geo: string;
  countryCode: string;
  asn: string;
  org: string;
  ipType: "residential" | "mobile" | "datacenter" | "unknown";
  status: "loading" | "ready";
  usageLabel: string;
  state: SignalState;
  confidence?: number;
  tier?: string;
  method?: string;
  humanRatio?: number;
  flags: string[];
  fraudScore: number;
  blacklist: string[];
  openPorts: number[];
  exposure: string[];
  present: boolean;
}

export interface RTTReadout {
  clientMinMS?: number;
  serverTCPMS?: number;
  deltaMS?: number;
  jitterMS?: number;
  peerMSS?: number;
  estMTU?: number;
  serverMTU?: number;
  mtuClamped?: boolean;
  ipVersion?: number;
  characteristics: ConnectionCharacteristic[];
}

export interface ReportModel {
  scenario: ScanScenario;
  score: number;
  riskLevel: RiskLevel;
  verdict: { state: SignalState; headline: string; sub: string };
  groups: ReportGroup[];
  identity: IdentityReadout;
  findings: Finding[];
  consistency: ReadoutRow[];
  leaks: ReadoutRow[];
  environment: ReadoutRow[];
  rtt?: RTTReadout;
  meta: {
    scanId: string;
    processedAt: string;
    processingMs: number;
    source: string;
    tier: string;
    httpVersion: string;
    detectMethod: string;
  };
}

function normalizeHumanRatio(v?: number): number | undefined {
  if (typeof v !== "number" || !Number.isFinite(v)) return undefined;
  return Math.max(0, Math.min(100, v));
}

function buildIdentity(
  a: IdentityAnalysis,
  intel: IPIntel | undefined,
  locale: Locale,
): IdentityReadout {
  const m = getMessages(locale);
  const present = !!(intel && intel.ip && intel.country !== "LOCAL");
  const flags: string[] = [];
  if (intel?.is_vpn) flags.push("VPN");
  if (intel?.is_proxy) flags.push("Proxy");
  if (intel?.is_tor || a.is_tor) flags.push("Tor");
  if (intel?.is_relay) flags.push("Relay");

  let state: SignalState;
  if (flags.length > 0 || a.blacklist_hits.length > 0 || a.is_tor_exit) {
    state = "leak";
  } else if (a.verdict === "datacenter_fail") {
    state = "contradiction";
  } else if (a.verdict === "gray_unknown" || !present) {
    state = "unknown";
  } else {
    state = "coherent";
  }

  const primaryPlace = (intel?.city || intel?.region || "").trim();
  const countryPlace = (intel?.country_name || intel?.country || "").trim();
  const geoParts: string[] = [];
  if (primaryPlace) geoParts.push(primaryPlace);
  if (countryPlace && countryPlace.toLowerCase() !== primaryPlace.toLowerCase()) {
    geoParts.push(countryPlace);
  }
  const ipType = a.ip_type ?? "unknown";

  return {
    ip: intel?.ip || "",
    masked: maskIP(intel?.ip || ""),
    geo: geoParts.join(" · ") || m.common.unknownLocation,
    countryCode:
      intel?.country && intel.country !== "LOCAL" ? intel.country : "",
    asn: intel?.asn || "—",
    org: dnsProvider(intel?.isp ?? "") || dnsProvider(intel?.org ?? "") || "—",
    ipType,
    status: "ready",
    usageLabel: m.common.usageType[ipType] ?? ipType,
    state,
    confidence: intel?.confidence,
    tier: intel?.tier,
    method: intel?.detect_method,
    humanRatio: normalizeHumanRatio(intel?.human_ratio ?? a.human_ratio),
    flags,
    fraudScore: a.fraud_score,
    blacklist: a.blacklist_hits ?? [],
    openPorts: a.open_ports ?? [],
    exposure: a.exposure_tags ?? [],
    present,
  };
}

const GROUP_ORDER: ReportGroup["key"][] = [
  "identity",
  "consistency",
  "leak",
  "environment",
];

function buildGroups(dims: DimensionScore[], locale: Locale): ReportGroup[] {
  const labels = getMessages(locale).home.groups;
  const byKey = new Map(dims.map((d) => [d.key, d]));
  return GROUP_ORDER.map((key) => {
    const d = byKey.get(key);
    if (!d) {
      return {
        key,
        label: labels[key],
        score: 60,
        state: "unknown" as SignalState,
        passed: 0,
        total: 1,
      };
    }
    const state = d.state as SignalState;
    return {
      key,
      label: labels[key],
      score: d.score,
      state,
      passed: d.passed,
      total: d.total,
    };
  });
}

function buildFindings(
  a: ScanAnalysis,
  id: IdentityReadout,
  scenario: ScanScenario,
  locale: Locale,
): Finding[] {
  const f = getMessages(locale).home.findings;
  const F: Finding[] = [];
  const push = (item: Finding) => F.push(item);

  const pv = a.privacy;
  if (pv.webrtc.status === "danger") {
    const webrtcCopy = pv.webrtc.reason === "reverse-split" ? f.webrtcReverseSplit : f.webrtcLeak;
    push({
      id: "webrtc-leak",
      severity: "leak",
      title: webrtcCopy.title,
      fact: webrtcCopy.fact(pv.webrtc.leaked_ips[0]),
      fix: webrtcCopy.fix,
    });
  }
  if (dnsVerdictState(pv.dns) === "suspected") {
    push({
      id: "dns-leak",
      severity: "leak",
      title: f.dnsLeak.title,
      fact: dnsVerdictNote(pv.dns.assessment, "current", locale),
      fix: f.dnsLeak.fix,
    });
  }
  if (pv.ipv6.status === "danger") {
    push({
      id: "ipv6-leak",
      severity: "leak",
      title: f.ipv6Leak.title,
      fact: f.ipv6Leak.fact(pv.ipv6.address),
      fix: f.ipv6Leak.fix,
    });
  }

  if (id.blacklist.length > 0) {
    push({
      id: "blacklist",
      severity: "leak",
      title: f.blacklist.title,
      fact: f.blacklist.fact(joinList(id.blacklist, locale)),
      fix: f.blacklist.fix,
    });
  }
  const proxyFlags = id.flags;
  if (proxyFlags.length > 0) {
    const joinedFlags = proxyFlags.join(" / ");
    push({
      id: "proxy-flag",
      severity: "leak",
      title: f.proxyFlag.title(joinedFlags),
      fact: f.proxyFlag.fact(joinedFlags),
      fix: f.proxyFlag.fix,
    });
  }
  if (a.identity.is_tor_exit) {
    push({
      id: "tor-exit",
      severity: "leak",
      title: f.torExit.title,
      fact: f.torExit.fact,
      fix: f.torExit.fix,
    });
  }
  if (
    scenario === "strict" &&
    a.identity.verdict === "datacenter_fail" &&
    proxyFlags.length === 0 &&
    id.blacklist.length === 0
  ) {
    push({
      id: "datacenter",
      severity: "caution",
      title: f.datacenter.title,
      fact: f.datacenter.fact(id.org),
      fix: f.datacenter.fix,
    });
  }
  if (a.identity.open_proxy_port) {
    push({
      id: "open-proxy-port",
      severity: "contradiction",
      title: f.openProxyPort.title,
      fact: f.openProxyPort.fact,
      fix: f.openProxyPort.fix,
    });
  }
  if (a.identity.fraud_score > 50) {
    push({
      id: "fraud-score",
      severity: "caution",
      title: f.fraudScore.title(a.identity.fraud_score),
      fact: f.fraudScore.fact(a.identity.fraud_score),
      fix: f.fraudScore.fix,
    });
  }

  const tz = a.consistency.timezone;
  if (scenario === "strict" && !tz.passed) {
    push({
      id: "tz-mismatch",
      severity: "contradiction",
      title: f.tzMismatch.title,
      fact: f.tzMismatch.fact(
        formatTimezoneZone(tz.browser, locale),
        formatTimezoneZone(tz.expected, locale),
      ),
      fix: f.tzMismatch.fix,
    });
  }
  const lang = a.consistency.language;
  if (scenario === "strict" && !lang.passed) {
    const ipCountryDisplay =
      lang.ip_country && /^[A-Z]{2}$/i.test(lang.ip_country)
        ? countryName(lang.ip_country, locale)
        : lang.ip_country || "?";
    push({
      id: "lang-mismatch",
      severity: "contradiction",
      title: f.langMismatch.title,
      fact: f.langMismatch.fact(
        languageName(lang.browser, locale),
        ipCountryDisplay,
        languageName((lang.expected || [])[0] || "", locale),
      ),
      fix: f.langMismatch.fix,
    });
  }
  const fp = a.fingerprint;
  if (a.automation.webdriver_detected) {
    push({
      id: "webdriver",
      severity: "leak",
      title: f.webdriver.title,
      fact: f.webdriver.fact,
      fix: f.webdriver.fix,
    });
  }
  if (fp.webgl.status === "mismatch") {
    const mismatchFact =
      fp.webgl.mismatch_code === "apple_d3d_nvidia"
        ? f.webglMismatch.appleD3dNvidia
        : fp.webgl.mismatch_code === "ios_angle"
          ? f.webglMismatch.iosAngle
          : locale === "zh-CN" && fp.webgl.mismatch_reason
            ? fp.webgl.mismatch_reason
            : f.webglMismatch.fallback;
    push({
      id: "webgl-mismatch",
      severity: "contradiction",
      title: f.webglMismatch.title,
      fact: mismatchFact,
      fix: f.webglMismatch.fix,
    });
  }
  if (scenario === "strict" && (fp.canvas.is_hooked || fp.audio.is_hooked)) {
    const which = [
      fp.canvas.is_hooked && "Canvas",
      fp.audio.is_hooked && "Audio",
    ]
      .filter(Boolean)
      .join(" / ");
    push({
      id: "fp-hooked",
      severity: "caution",
      title: f.fpHooked.title(which),
      fact: f.fpHooked.fact(which),
      fix: f.fpHooked.fix,
    });
  }
  if (
    a.automation.no_desktop_plugins ||
    a.automation.automation_traces.some((t) => /plugins_length/.test(t))
  ) {
    push({
      id: "no-plugins",
      severity: "info",
      title: f.noPlugins.title,
      fact: f.noPlugins.fact,
      fix: f.noPlugins.fix,
    });
  }

  const rank: Record<Severity, number> = {
    leak: 0,
    contradiction: 1,
    caution: 2,
    info: 3,
  };
  return F.sort((x, y) => rank[x.severity] - rank[y.severity]);
}

function consistencyRows(
  a: ScanAnalysis,
  scenario: ScanScenario,
  locale: Locale,
): ReadoutRow[] {
  const r = getMessages(locale).home.readout;
  const tz = a.consistency.timezone;
  const lang = a.consistency.language;
  const tzUnknown = ["unknown", "-", ""].includes(tz.expected);
  const daily = scenario === "daily";
  const tzBrowser = formatTimezoneZone(tz.browser, locale);
  const tzExpected = formatTimezoneZone(tz.expected, locale);
  const langBrowser = languageName(lang.browser, locale);
  const matchedExpected =
    lang.passed && lang.browser
      ? (lang.expected || []).find((exp) => {
          const b = lang.browser.toLowerCase();
          const e = exp.toLowerCase();
          return b === e || b.startsWith(`${e}-`);
        })
      : undefined;
  const langExpected = languageName(matchedExpected || (lang.expected || [])[0] || "", locale);
  return [
    {
      key: "timezone",
      label: r.timezone,
      value: `${tzBrowser} ↔ ${tzExpected}`,
      state: tzUnknown ? "unknown" : tz.passed ? "coherent" : daily ? "unknown" : "contradiction",
      compare: { browser: tzBrowser, ip: tzExpected },
    },
    {
      key: "language",
      label: r.language,
      value: `${langBrowser} ↔ ${langExpected}`,
      state: lang.passed ? "coherent" : daily ? "unknown" : "contradiction",
      compare: {
        browser: langBrowser,
        ip: langExpected,
      },
    },
  ];
}

function dnsVerdictState(dns: ScanAnalysis["privacy"]["dns"]): DNSVerdict["state"] {
  if (dns.assessment?.policy_version === DNS_POLICY_VERSION) return dns.assessment.state;
  return dns.status === "blocked" ? "unobserved" : "unknown";
}

function leakRows(a: ScanAnalysis, exit: DNSExit | null, locale: Locale): ReadoutRow[] {
  const r = getMessages(locale).home.readout;
  const dnsLabels = getDnsStatus(locale);
  const pv = a.privacy;
  const exitIP = exit?.ip ?? "";
  const webrtcState: SignalState =
    pv.webrtc.status === "safe" || pv.webrtc.status === "blocked"
      ? "coherent"
      : pv.webrtc.status === "danger"
        ? "leak"
        : "unknown";
  const dnsState = dnsVerdictState(pv.dns);
  const ipv6State: SignalState =
    pv.ipv6.status === "danger"
      ? "leak"
      : pv.ipv6.status === "unknown"
        ? "unknown"
        : "coherent";
  const webrtcValue = (() => {
    if (pv.webrtc.status === "blocked") return r.webrtcNone;
    if (pv.webrtc.status === "safe") return r.webrtcMatch;
    const extra = pv.webrtc.leaked_ips ?? [];
    if (extra.length === 0) return r.webrtcNone;
    if (pv.webrtc.reason === "reverse-split") return r.webrtcReverseSplit;
    return r.webrtcMismatch;
  })();

  const ipv6Kind: NonNullable<ReadoutRow["meta"]>["ipv6Kind"] =
    pv.ipv6.status === "danger"
      ? "leaked"
      : pv.ipv6.status === "unknown"
        ? "unknown"
        : !pv.ipv6.address || pv.ipv6.status === "disabled"
          ? "none"
          : exitIP && isIPv6Address(exitIP) && sameIP(pv.ipv6.address, exitIP)
            ? "matches-exit"
            : "dual-stack";

  const ipv6Value =
    ipv6Kind === "leaked"
      ? r.ipv6Mismatch
      : ipv6Kind === "unknown"
        ? getMessages(locale).ipv6.status.unknown
        : ipv6Kind === "none"
          ? r.ipv6None
          : ipv6Kind === "matches-exit"
            ? r.ipv6Match
            : r.ipv6Dual;

  return [
    {
      key: "webrtc",
      label: "WebRTC",
      value: webrtcValue,
      state: webrtcState,
    },
    {
      key: "dns",
      label: "DNS",
      value: dnsLabels[dnsState].title,
      state: dnsState === "suspected" ? "leak" : dnsState === "clear" ? "coherent" : "unknown",
      href: dnsState === "suspected" ? localePath("/dns-leak-test/", locale) : undefined,
      hint:
        pv.dns.assessment?.policy_version === DNS_POLICY_VERSION
          ? dnsVerdictNote(pv.dns.assessment, "current", locale)
          : undefined,
      meta: { dnsVerdict: pv.dns.assessment },
    },
    {
      key: "ipv6",
      label: "IPv6",
      value: ipv6Value,
      state: ipv6State,
      href: pv.ipv6.status === "danger" ? localePath("/ipv6-leak-test/", locale) : undefined,
      meta: { ipv6Kind },
    },
  ];
}

function environmentRows(
  a: ScanAnalysis,
  c: AllCollectorResults,
  scenario: ScanScenario,
  locale: Locale,
): ReadoutRow[] {
  const r = getMessages(locale).home.readout;
  const fp = a.fingerprint;
  const nav = c.environment.navigator?.data;
  const daily = scenario === "daily";
  return [
    {
      key: "webgl",
      label: r.gpu,
      value:
        fp.webgl.status === "mismatch"
          ? r.gpuMismatch
          : fp.webgl.status === "match"
            ? r.gpuMatch
            : r.gpuUnknown,
      state:
        fp.webgl.status === "mismatch"
          ? "contradiction"
          : fp.webgl.status === "match"
            ? "coherent"
            : "unknown",
    },
    {
      key: "canvas",
      label: r.canvas,
      value: fp.canvas.is_hooked ? r.fpHooked : r.fpNormal,
      state: fp.canvas.is_hooked ? (daily ? "unknown" : "contradiction") : "coherent",
    },
    {
      key: "audio",
      label: r.audio,
      value: fp.audio.is_hooked ? r.fpHooked : r.fpNormal,
      state: fp.audio.is_hooked ? (daily ? "unknown" : "contradiction") : "coherent",
    },
    {
      key: "automation",
      label: r.automation,
      value: a.automation.webdriver_detected
        ? r.autoDetected
        : a.automation.overall_status === "suspicious"
          ? r.autoSuspicious
          : r.autoNone,
      state: a.automation.webdriver_detected
        ? "leak"
        : a.automation.overall_status === "suspicious"
          ? "contradiction"
          : "coherent",
    },
    {
      key: "hardware",
      label: r.hardware,
      value: r.hardwareValue(
        nav?.hardwareConcurrency ?? "?",
        nav?.deviceMemory ?? "?",
        nav?.maxTouchPoints ?? 0,
      ),
      state: "coherent",
    },
  ];
}

function buildRTT(a: ScanAnalysis): RTTReadout | undefined {
  const r = a.rtt;
  if (!r || r.status === "unavailable") return undefined;
  const pos = (x?: number) => (typeof x === "number" && x > 0 ? x : undefined);
  const deltaMS =
    typeof r.delta_min_ms === "number" && Number.isFinite(r.delta_min_ms)
      ? r.delta_min_ms <= 0
        ? 0
        : r.delta_min_ms
      : undefined;
  const clientMinMS = pos(r.client_min_ms);
  const serverTCPMS = pos(r.server_tcp_min_ms) ?? pos(r.server_tcp_ms);
  const peerMSS = pos(r.peer_mss);
  const estMTU = pos(r.est_mtu);
  const ipVersion = r.ip_version === 4 || r.ip_version === 6 ? r.ip_version : undefined;
  if (clientMinMS == null && serverTCPMS == null && estMTU == null) return undefined;
  const characteristics: ConnectionCharacteristic[] =
    Array.isArray(r.characteristics) && r.characteristics.length > 0
      ? r.characteristics
      : ["inconclusive"];
  return {
    clientMinMS,
    serverTCPMS,
    deltaMS,
    jitterMS: pos(r.client_jitter_ms),
    peerMSS,
    estMTU,
    serverMTU: pos(r.server_mtu),
    mtuClamped: r.mtu_clamped === true ? true : undefined,
    ipVersion,
    characteristics,
  };
}

function buildVerdict(
  groups: ReportGroup[],
  scenario: ScanScenario,
  locale: Locale,
): ReportModel["verdict"] {
  const v = getMessages(locale).home.verdict;
  const has = (s: SignalState) => groups.some((g) => g.state === s);
  if (has("leak")) {
    return {
      state: "leak",
      headline: v.leak.headline,
      sub: v.leak.sub,
    };
  }
  if (has("contradiction")) {
    return {
      state: "contradiction",
      headline: v.contradiction.headline,
      sub: v.contradiction.sub,
    };
  }
  const unknownHeavy = groups.filter((g) => g.state === "unknown").length >= 2;
  if (unknownHeavy) {
    return {
      state: "unknown",
      headline: v.unknown.headline,
      sub: v.unknown.sub,
    };
  }
  return {
    state: "coherent",
    headline: v.coherent.headline,
    sub: scenario === "daily" ? v.coherent.subDaily : v.coherent.subStrict,
  };
}

export function buildReport(
  resp: ScanResponse,
  collectors: AllCollectorResults,
  locale: Locale = "zh-CN",
): ReportModel {
  const a = resp.analysis;
  const scenario: ScanScenario = resp.scenario === "daily" ? "daily" : "strict";
  const groups = buildGroups(resp.dimensions ?? [], locale);
  const identity = buildIdentity(a.identity, resp.server_data.ip_intel, locale);
  const exit = resp.server_data.ip_intel ? browserExit(resp.server_data.ip_intel) : null;
  const idGroup = groups.find((g) => g.key === "identity");
  if (idGroup) identity.state = idGroup.state;
  const verdict = buildVerdict(groups, scenario, locale);

  return {
    scenario,
    score: resp.score,
    riskLevel: resp.risk_level,
    verdict,
    groups,
    identity,
    findings: buildFindings(a, identity, scenario, locale),
    consistency: consistencyRows(a, scenario, locale),
    leaks: leakRows(a, exit, locale),
    environment: environmentRows(a, collectors, scenario, locale),
    rtt: buildRTT(a),
    meta: {
      scanId: resp.scan_id,
      processedAt: resp.processed_at,
      processingMs: resp.processing_time_ms,
      source: resp.server_data.ip_intel?.source || "—",
      tier: resp.server_data.ip_intel?.tier || "—",
      httpVersion: resp.server_data.http_version || "—",
      detectMethod: resp.server_data.ip_intel?.detect_method || "—",
    },
  };
}

export function stateVar(state: SignalState): string {
  switch (state) {
    case "coherent":
      return "var(--sig)";
    case "contradiction":
      return "var(--warn)";
    case "leak":
      return "var(--leak)";
    default:
      return "var(--unknown)";
  }
}

export function stateSoft(state: SignalState): string {
  switch (state) {
    case "coherent":
      return "var(--sig-soft)";
    case "contradiction":
      return "var(--warn-soft)";
    case "leak":
      return "var(--leak-soft)";
    default:
      return "var(--unknown-soft)";
  }
}
