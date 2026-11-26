
import type { AllCollectorResults } from "./collectors/types";
import type { DeviceSignals } from "./collectors/environment/device";
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
} from "./types";

import { browserExit, DNS_POLICY_VERSION, dnsVerdictNote, dnsStatus } from "./dnsPresentation";
import { isIPv6Address, sameIP } from "./ip";
import { maskIP } from "./ipDisplay";

export type SignalState = "coherent" | "contradiction" | "leak" | "unknown";
export type Severity = "leak" | "contradiction" | "caution" | "info";

export interface Finding {
  id: string;
  severity: Severity;
  title: string;
  fact: string;
  meaning: string;
  fix: string;
  platformNote?: string;
}

export interface ReportGroup {
  key: "identity" | "consistency" | "leak" | "environment";
  label: string;
  score: number;
  state: SignalState;
  passed: number;
  total: number;
  note: string;
}

export interface ReadoutRow {
  label: string;
  value: string;
  state: SignalState;
  href?: string;
  hint?: string;
  compare?: { browser: string; ip: string };
}

export interface IdentityReadout {
  ip: string;
  masked: string;
  geo: string;
  countryCode: string;
  asn: string;
  org: string;
  usageLabel: string;
  state: SignalState;
  confidence?: number;
  tier?: string;
  method?: string;
  humanRatio?: number;
  ceilingNote?: string;
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

const USAGE_LABEL: Record<string, string> = {
  residential: "普通网络",
  mobile: "移动网络",
  datacenter: "机房 IP",
  unknown: "类型未定",
};

const LANG_NAME: Record<string, string> = {
  zh: "简体中文",
  "zh-tw": "繁体中文",
  "zh-hk": "繁体中文",
  en: "英语",
  ja: "日语",
  ko: "韩语",
  fr: "法语",
  de: "德语",
  es: "西班牙语",
  pt: "葡萄牙语",
  ru: "俄语",
  it: "意大利语",
  vi: "越南语",
  th: "泰语",
  id: "印尼语",
  ms: "马来语",
  hi: "印地语",
};
function langName(code: string): string {
  if (!code) return "未知";
  const c = code.toLowerCase();
  return LANG_NAME[c] || LANG_NAME[c.split("-")[0]] || code;
}

const CN_NUM = [
  "零",
  "一",
  "二",
  "三",
  "四",
  "五",
  "六",
  "七",
  "八",
  "九",
  "十",
  "十一",
  "十二",
  "十三",
];
function offsetToZone(mins: number): string {
  if (mins === 0) return "零时区";
  const abs = Math.abs(mins);
  const h = Math.floor(abs / 60);
  const m = abs % 60;
  const hz = CN_NUM[h] ?? String(h);
  const dir = mins > 0 ? "东" : "西";
  return m === 0 ? `${dir}${hz}区` : `${dir}${hz}区${m}分`;
}

function tzToZone(tz: string): string {
  if (!tz || tz === "unknown" || tz === "-") return "未知";
  if (tz === "UTC" || tz === "GMT" || tz === "Z") return "零时区";
  const m = tz.match(/^([+-])(\d{2}):?(\d{2})$/);
  if (m) {
    const mins =
      (parseInt(m[2], 10) * 60 + parseInt(m[3], 10)) * (m[1] === "-" ? -1 : 1);
    return offsetToZone(mins);
  }
  try {
    const name =
      new Intl.DateTimeFormat("en-US", {
        timeZone: tz,
        timeZoneName: "longOffset",
      })
        .formatToParts(new Date())
        .find((p) => p.type === "timeZoneName")?.value || "";
    const mm = name.match(/GMT([+-])(\d{1,2})(?::(\d{2}))?/);
    if (mm) {
      const mins =
        (parseInt(mm[2], 10) * 60 + parseInt(mm[3] || "0", 10)) *
        (mm[1] === "-" ? -1 : 1);
      return offsetToZone(mins);
    }
  } catch {
  }
  return tz;
}

function normalizeHumanRatio(v?: number): number | undefined {
  if (typeof v !== "number" || !Number.isFinite(v)) return undefined;
  return Math.max(0, Math.min(100, v));
}

function buildIdentity(
  a: IdentityAnalysis,
  intel: IPIntel | undefined,
): IdentityReadout {
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

  const geoParts = [intel?.city, intel?.country_name || intel?.country].filter(
    Boolean,
  );
  const ceilingNote: string | undefined = undefined;

  return {
    ip: intel?.ip || "",
    masked: maskIP(intel?.ip || ""),
    geo: geoParts.join(" · ") || "未知位置",
    countryCode:
      intel?.country && intel.country !== "LOCAL" ? intel.country : "",
    asn: intel?.asn || "—",
    org: intel?.org || intel?.isp || "—",
    usageLabel: USAGE_LABEL[a.ip_type] ?? a.ip_type,
    state,
    confidence: intel?.confidence,
    tier: intel?.tier,
    method: intel?.detect_method,
    humanRatio: normalizeHumanRatio(intel?.human_ratio ?? a.human_ratio),
    ceilingNote,
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

const GROUP_LABEL: Record<ReportGroup["key"], string> = {
  identity: "网络身份",
  consistency: "环境一致性",
  leak: "DNS/IP 泄露",
  environment: "环境指纹",
};

function groupNote(key: ReportGroup["key"], state: SignalState, scenario: ScanScenario): string {
  switch (key) {
    case "identity":
      return state === "leak"
        ? "出口 IP 携带代理/黑名单特征"
        : state === "contradiction"
          ? "机房 / 托管 IP"
          : state === "unknown"
            ? "类型无法高置信判定"
            : "出口正常";
    case "consistency":
      if (scenario === "daily") return "日常模式下仅作展示";
      return state === "coherent"
        ? "时区 / 语言与 IP 相符"
        : "存在与 IP 不符的环境项";
    case "environment":
      return state === "coherent"
        ? "指纹正常、无自动化痕迹"
        : "指纹 / 自动化存在异常";
    default:
      return "";
  }
}

function buildGroups(dims: DimensionScore[], scenario: ScanScenario): ReportGroup[] {
  const byKey = new Map(dims.map((d) => [d.key, d]));
  return GROUP_ORDER.map((key) => {
    const d = byKey.get(key);
    if (!d) {
      return {
        key,
        label: GROUP_LABEL[key],
        score: 60,
        state: "unknown" as SignalState,
        passed: 0,
        total: 1,
        note: "",
      };
    }
    const state = d.state as SignalState;
    return {
      key,
      label: d.label || GROUP_LABEL[key],
      score: d.score,
      state,
      passed: d.passed,
      total: d.total,
      note: groupNote(key, state, scenario),
    };
  });
}

function buildFindings(a: ScanAnalysis, id: IdentityReadout, scenario: ScanScenario): Finding[] {
  const F: Finding[] = [];
  const push = (f: Finding) => F.push(f);

  const pv = a.privacy;
  if (pv.webrtc.status === "danger" && !isExpectedDualStackWebRTC(pv, id.ip)) {
    push({
      id: "webrtc-leak",
      severity: "leak",
      title: "WebRTC 公网地址与出口 IP 不一致",
      fact: `网页可获取额外公网地址${pv.webrtc.leaked_ips[0] ? `（${pv.webrtc.leaked_ips[0]}）` : ""}`,
      meaning: "若正在使用代理，这可能暴露另一条公网出口。",
      fix: "检查代理是否覆盖 WebRTC，或关闭浏览器 WebRTC 公网候选。",
    });
  }
  if (dnsVerdictState(pv.dns) === "suspected") {
    push({
      id: "dns-leak",
      severity: "leak",
      title: "DNS 泄露",
      fact: dnsVerdictNote(pv.dns.assessment),
      meaning: "国内运营商参与了本次域名解析。",
      fix: "在 DNS 泄露检测页复测，核对测到的解析网络。",
    });
  }
  if (pv.ipv6.status === "danger") {
    push({
      id: "ipv6-leak",
      severity: "leak",
      title: "IPv6 与出口 IP 不一致",
      fact: `检测到全局 IPv6 地址${pv.ipv6.address ? `（${pv.ipv6.address}）` : ""}`,
      meaning: "IPv6 与当前出口不一致，可能形成另一条公网出口。",
      fix: "在 IPv6 泄露检测页复测，检查代理是否接管 IPv6 或在路由层关闭 IPv6 直连。",
    });
  }

  if (id.blacklist.length > 0) {
    push({
      id: "blacklist",
      severity: "leak",
      title: "出口 IP 命中黑名单",
      fact: `命中 ${id.blacklist.join("、")}`,
      meaning: "被公开黑名单收录的 IP 会被大量站点拒绝或加验。",
      fix: "立即更换出口 IP。",
    });
  }
  const proxyFlags = id.flags.filter((f) => f !== "Hosting");
  if (proxyFlags.length > 0) {
    push({
      id: "proxy-flag",
      severity: "leak",
      title: `IP 被标记为 ${proxyFlags.join(" / ")}`,
      fact: `第三方数据源将该 IP 标记为 ${proxyFlags.join(" / ")}`,
      meaning: "被标记的匿名网络出口会被直接归为高风险。",
      fix: "更换未被标记的网络出口。",
    });
  }
  if (a.identity.is_tor_exit) {
    push({
      id: "tor-exit",
      severity: "leak",
      title: "出口为 Tor 出口节点",
      fact: "该 IP 在公开的 Tor 出口列表中",
      meaning: "Tor 出口公开可查，几乎所有平台都会拦。",
      fix: "改用普通网络出口。",
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
      title: "出口为机房 IP",
      fact: `服务商 ${id.org}`,
      meaning: "机房 IP 常被平台风控识别为非自然流量。",
      fix: "如需更高可信度，改用更稳定的网络出口。",
    });
  }
  if (a.identity.open_proxy_port) {
    push({
      id: "open-proxy-port",
      severity: "contradiction",
      title: "IP 开放代理端口",
      fact: "扫描到该 IP 开放了常见代理端口",
      meaning: "开放代理端口是主动暴露的代理特征。",
      fix: "关闭对外代理端口，或更换 IP。",
    });
  }
  if (a.identity.fraud_score > 50) {
    push({
      id: "fraud-score",
      severity: "caution",
      title: `IP 欺诈评分偏高（${a.identity.fraud_score}/100）`,
      fact: `第三方数据源给出的欺诈评分为 ${a.identity.fraud_score}`,
      meaning: "高欺诈分的 IP 更容易被风控预先标记。",
      fix: "更换信誉更好的 IP。",
    });
  }

  const tz = a.consistency.timezone;
  if (scenario === "strict" && !tz.passed) {
    push({
      id: "tz-mismatch",
      severity: "contradiction",
      title: "浏览器时区与 IP 归属地冲突",
      fact: `浏览器时区为${tzToZone(tz.browser)}，IP 归属地时区为${tzToZone(tz.expected)}`,
      meaning: "时区对不上是最常见的环境破绽，单项就足以拉低可信度。",
      fix: "把系统时区改成与 IP 归属地一致。",
    });
  }
  const lang = a.consistency.language;
  if (scenario === "strict" && !lang.passed) {
    push({
      id: "lang-mismatch",
      severity: "contradiction",
      title: "系统语言与 IP 国家不匹配",
      fact: `浏览器语言 ${langName(lang.browser)}，IP 国家（${lang.ip_country || "?"}）当地常用 ${langName((lang.expected || [])[0] || "")}`,
      meaning: "语言与地理位置对不上，是环境不一致的直接证据。",
      fix: "把浏览器首选语言调成 IP 归属地对应语言。",
    });
  }
  const fp = a.fingerprint;
  if (a.automation.webdriver_detected) {
    push({
      id: "webdriver",
      severity: "leak",
      title: "检测到自动化控制标记",
      fact: "navigator.webdriver = true，暴露了自动化控制",
      meaning: "这是自动化最直接的自证，平台可据此直接封禁。",
      fix: "使用未打标的浏览器或反检测方案。",
    });
  }
  if (fp.webgl.status === "mismatch") {
    push({
      id: "webgl-mismatch",
      severity: "contradiction",
      title: "显卡信息与系统冲突",
      fact:
        fp.webgl.mismatch_reason ||
        "显卡信息与操作系统 / 硬件声明物理上不可能共存",
      meaning: "硬件指纹自相矛盾，是伪装环境最难圆的破绽。",
      fix: "确保显卡信息与声称的系统、硬件逻辑一致。",
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
      title: `${which} 指纹被篡改`,
      fact: `${which} 的原生方法被改写（反指纹工具痕迹）`,
      meaning: "过度伪造本身就可疑——完全屏蔽比留噪音更扎眼。",
      fix: "指纹保护改用 noise 模式，避免完全屏蔽。",
    });
  }
  for (const t of a.automation.automation_traces) {
    if (/plugins_length/.test(t)) {
      push({
        id: "no-plugins",
        severity: "info",
        title: "桌面浏览器无插件",
        fact: "桌面浏览器插件列表为空，偏离常见环境",
        meaning: "单项风险低，但与其他异常叠加会抬高可疑度。",
        fix: "通常无需处理。",
      });
      break;
    }
  }

  const rank: Record<Severity, number> = {
    leak: 0,
    contradiction: 1,
    caution: 2,
    info: 3,
  };
  return F.sort((x, y) => rank[x.severity] - rank[y.severity]);
}

function consistencyRows(a: ScanAnalysis, scenario: ScanScenario): ReadoutRow[] {
  const tz = a.consistency.timezone;
  const lang = a.consistency.language;
  const tzUnknown = ["unknown", "-", ""].includes(tz.expected);
  const daily = scenario === "daily";
  return [
    {
      label: "时区",
      value: `${tzToZone(tz.browser)} ↔ ${tzToZone(tz.expected)}`,
      state: tzUnknown ? "unknown" : tz.passed ? "coherent" : daily ? "unknown" : "contradiction",
      compare: { browser: tzToZone(tz.browser), ip: tzToZone(tz.expected) },
      hint: tzUnknown ? "IP 时区未知" : !tz.passed && daily ? "日常模式下仅作展示" : undefined,
    },
    {
      label: "语言",
      value: `${langName(lang.browser)} ↔ ${langName((lang.expected || [])[0] || "")}`,
      state: lang.passed ? "coherent" : daily ? "unknown" : "contradiction",
      compare: {
        browser: langName(lang.browser),
        ip: langName((lang.expected || [])[0] || ""),
      },
      hint: !lang.passed && daily ? "日常模式下仅作展示" : undefined,
    },
  ];
}

function allSameAddress(addrs: string[], target: string): boolean {
  return addrs.length > 0 && addrs.every((ip) => sameIP(ip, target));
}

export function isExpectedDualStackWebRTC(
  pv: ScanAnalysis["privacy"],
  exitIP = "",
): boolean {
  const extra = pv.webrtc.leaked_ips ?? [];
  if (extra.length === 0 || pv.webrtc.status === "danger" || pv.ipv6.status === "danger") {
    return false;
  }
  if (pv.ipv6.address && allSameAddress(extra, pv.ipv6.address)) return true;
  if (exitIP && !isIPv6Address(exitIP) && extra.every((ip) => isIPv6Address(ip)))
    return true;
  if (
    exitIP &&
    isIPv6Address(exitIP) &&
    extra.every((ip) => !isIPv6Address(ip))
  ) {
    return true;
  }
  return false;
}

function dnsVerdictState(dns: ScanAnalysis["privacy"]["dns"]): DNSVerdict["state"] {
  if (dns.assessment?.policy_version === DNS_POLICY_VERSION) return dns.assessment.state;
  return dns.status === "blocked" ? "unobserved" : "unknown";
}

function leakRows(a: ScanAnalysis, exit: DNSExit | null): ReadoutRow[] {
  const pv = a.privacy;
  const exitIP = exit?.ip ?? "";
  const dualStack = isExpectedDualStackWebRTC(pv, exitIP);
  const webrtcState: SignalState =
    dualStack || pv.webrtc.status === "safe" || pv.webrtc.status === "blocked"
      ? "coherent"
      : pv.webrtc.status === "danger"
        ? "leak"
        : "unknown";
  const dnsState = dnsVerdictState(pv.dns);
  const ipv6State: SignalState =
    pv.ipv6.status === "danger" ? "leak" : "coherent";
  const webrtcValue = (() => {
    if (pv.webrtc.status === "blocked") return "未检测到公网地址";
    if (pv.webrtc.status === "safe") return "与出口 IP 一致";
    if (dualStack) return "检测到 IPv6 双栈";
    const extra = pv.webrtc.leaked_ips ?? [];
    if (extra.length === 0) return "未检测到公网地址";
    return "公网地址与出口 IP 不一致";
  })();

  const ipv6Value = (() => {
    if (pv.ipv6.status === "danger") return "IPv6 与出口 IP 不一致";
    if (!pv.ipv6.address || pv.ipv6.status === "disabled")
      return "未检测到 IPv6";
    if (
      exitIP &&
      isIPv6Address(exitIP) &&
      sameIP(pv.ipv6.address, exitIP)
    )
      return "与出口 IP 一致";
    return "检测到 IPv6 双栈";
  })();

  return [
    {
      label: "WebRTC",
      value: webrtcValue,
      state: webrtcState,
    },
    {
      label: "DNS",
      value: dnsStatus[dnsState].title,
      state: dnsState === "suspected" ? "leak" : dnsState === "clear" ? "coherent" : "unknown",
      href: dnsState === "suspected" ? "/dns-leak-test/" : undefined,
      hint: pv.dns.assessment?.policy_version === DNS_POLICY_VERSION ? dnsVerdictNote(pv.dns.assessment) : undefined,
    },
    {
      label: "IPv6",
      value: ipv6Value,
      state: ipv6State,
      href: pv.ipv6.status === "danger" ? "/ipv6-leak-test/" : undefined,
    },
  ];
}

function environmentRows(
  a: ScanAnalysis,
  c: AllCollectorResults,
  d: DeviceSignals,
  scenario: ScanScenario,
): ReadoutRow[] {
  const fp = a.fingerprint;
  const nav = c.environment.navigator?.data;
  const daily = scenario === "daily";
  return [
    {
      label: "显卡",
      value:
        fp.webgl.status === "mismatch"
          ? "与系统冲突"
          : fp.webgl.status === "match"
            ? "与系统相符"
            : "无法比对",
      state:
        fp.webgl.status === "mismatch"
          ? "contradiction"
          : fp.webgl.status === "match"
            ? "coherent"
            : "unknown",
    },
    {
      label: "Canvas 指纹",
      value: fp.canvas.is_hooked ? "被篡改" : "正常",
      state: fp.canvas.is_hooked ? (daily ? "unknown" : "contradiction") : "coherent",
      hint: fp.canvas.is_hooked && daily ? "日常模式下仅作展示" : undefined,
    },
    {
      label: "Audio 指纹",
      value: fp.audio.is_hooked ? "被篡改" : "正常",
      state: fp.audio.is_hooked ? (daily ? "unknown" : "contradiction") : "coherent",
      hint: fp.audio.is_hooked && daily ? "日常模式下仅作展示" : undefined,
    },
    {
      label: "自动化",
      value: a.automation.webdriver_detected
        ? "检测到控制标记"
        : a.automation.overall_status === "suspicious"
          ? "有可疑痕迹"
          : "未检测到",
      state: a.automation.webdriver_detected
        ? "leak"
        : a.automation.overall_status === "suspicious"
          ? "contradiction"
          : "coherent",
    },
    {
      label: "硬件",
      value: `${nav?.hardwareConcurrency ?? "?"} 核 / ${nav?.deviceMemory ?? "?"}GB / ${d.maxTouchPoints} 触控点`,
      state: "coherent",
      hint: "CPU / 内存 / 触控",
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
  const out: RTTReadout = {
    clientMinMS: pos(r.client_min_ms),
    serverTCPMS: pos(r.server_tcp_min_ms) ?? pos(r.server_tcp_ms),
    deltaMS,
    jitterMS: pos(r.client_jitter_ms),
  };
  if (out.clientMinMS == null && out.serverTCPMS == null) return undefined;
  return out;
}

function buildVerdict(groups: ReportGroup[], scenario: ScanScenario): ReportModel["verdict"] {
  const has = (s: SignalState) => groups.some((g) => g.state === s);
  if (has("leak")) {
    return {
      state: "leak",
      headline: "网络身份存在异常",
      sub: "存在公网地址不一致或环境特征被标记的风险。",
    };
  }
  if (has("contradiction")) {
    return {
      state: "contradiction",
      headline: "环境特征与 IP 冲突",
      sub: "网络身份与环境特征对不上，可能被识别为异常环境。",
    };
  }
  const unknownHeavy = groups.filter((g) => g.state === "unknown").length >= 2;
  if (unknownHeavy) {
    return {
      state: "unknown",
      headline: "部分项无法判定",
      sub: "关键信号缺失或处于灰区，暂时无法给出确定结论。",
    };
  }
  return {
    state: "coherent",
    headline: "未检测到明显异常",
    sub:
      scenario === "daily"
        ? "未发现网络泄露或高危出口特征。"
        : "网络身份与环境特征基本一致。",
  };
}

export function buildReport(
  resp: ScanResponse,
  collectors: AllCollectorResults,
  device: DeviceSignals,
): ReportModel {
  const a = resp.analysis;
  const scenario: ScanScenario = resp.scenario === "daily" ? "daily" : "strict";
  const groups = buildGroups(resp.dimensions ?? [], scenario);
  const identity = buildIdentity(a.identity, resp.server_data.ip_intel);
  const exit = resp.server_data.ip_intel ? browserExit(resp.server_data.ip_intel) : null;
  const idGroup = groups.find((g) => g.key === "identity");
  if (idGroup) identity.state = idGroup.state;
  const verdict = buildVerdict(groups, scenario);

  return {
    scenario,
    score: resp.score,
    riskLevel: resp.risk_level,
    verdict,
    groups,
    identity,
    findings: buildFindings(a, identity, scenario),
    consistency: consistencyRows(a, scenario),
    leaks: leakRows(a, exit),
    environment: environmentRows(a, collectors, device, scenario),
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
