/**
 * 报告合成层：把后端 ScanResponse（+ 客户端采集）综合成「一份一致性 + 暴露体检报告」。
 *
 * 产品论点：IP 层无法直接证明用户身份，所以产品回答的是「网络与环境是否自洽」。
 * 本文件把四类信号
 * （出口身份 / 一致性 / 泄露 / 环境真实性）综合为：一句判定 + 破绽清单 + 四组读数。
 */

import type { AllCollectorResults } from "./collectors/types";
import type { DeviceSignals } from "./collectors/environment/device";
import type {
  ScanResponse,
  ScanAnalysis,
  IdentityAnalysis,
  IPIntel,
  RiskLevel,
  DimensionScore,
} from "./types";

// 四态语义（对应 global.css 的 --sig / --warn / --leak / --unknown）
export type SignalState = "coherent" | "contradiction" | "leak" | "unknown";
export type Severity = "leak" | "contradiction" | "caution" | "info";

export interface Finding {
  id: string;
  severity: Severity;
  title: string;
  fact: string; // 检测到什么（事实）
  meaning: string; // 意味着什么（风险）
  fix: string; // 怎么修
  platformNote?: string; // 降级后的场景知识：对哪类平台尤其致命
}

export interface ReportGroup {
  key: "identity" | "consistency" | "leak" | "environment";
  label: string;
  score: number; // 0-100 相干度
  state: SignalState;
  passed: number;
  total: number;
  note: string;
}

export interface ReadoutRow {
  label: string;
  value: string;
  state: SignalState;
  hint?: string;
  compare?: { browser: string; ip: string }; // 浏览器↔IP 对比行
}

export interface IdentityReadout {
  ip: string;
  masked: string;
  geo: string;
  countryCode: string; // ISO alpha-2，用于国旗
  asn: string;
  org: string;
  usageLabel: string;
  state: SignalState;
  confidence?: number;
  tier?: string;
  method?: string;
  humanRatio?: number;
  ceilingNote?: string;
  flags: string[]; // vpn/proxy/tor/relay/hosting 命中项
  fraudScore: number;
  blacklist: string[];
  openPorts: number[];
  exposure: string[];
  present: boolean; // ip_intel 是否有效（loopback/失败时为 false）
}

/**
 * RTT 物理延迟读数（Phase 1）：只呈现测量值，不判定不计分——阈值未标定前，
 * 任何「正常 / 异常」的说法都是拍脑袋。geo 系列刻意不进呈现层（violation 即判定）。
 */
export interface RTTReadout {
  clientMinMS?: number; // 浏览器↔服务器 全程（含代理腿）
  serverTCPMS?: number; // 出口↔服务器（nginx 内核实测）
  deltaMS?: number; // 两段之差 ≈ 浏览器↔出口 这一腿
  jitterMS?: number; // 客户端多次采样的抖动，悬停提示用
}

export interface ReportModel {
  score: number;
  riskLevel: RiskLevel;
  verdict: { state: SignalState; headline: string; sub: string };
  groups: ReportGroup[];
  identity: IdentityReadout;
  findings: Finding[];
  consistency: ReadoutRow[];
  leaks: ReadoutRow[];
  environment: ReadoutRow[];
  rtt?: RTTReadout; // 缺失 = 本次未测得（不渲染读数条）
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

// ---- 工具 ----

function maskIP(ip: string): string {
  if (!ip) return "—";
  const v4 = ip.split(".");
  if (v4.length === 4) return `${v4[0]}.${v4[1]}.···.···`;
  const v6 = ip.split(":");
  if (v6.length > 2) return `${v6[0]}:${v6[1]}:···`;
  return ip;
}

const USAGE_LABEL: Record<string, string> = {
  residential: "普通网络",
  mobile: "移动网络",
  datacenter: "机房 IP",
  unknown: "类型未定",
};

// 语言代码 → 中文名（按前缀匹配）
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

// UTC 偏移（分钟）→ 「东八区 / 西五区 / 零时区」
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

// 时区（IANA 名 / ±HH:MM 偏移）→ 中文「东N区」表述
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
    /* 无法解析则回退原串 */
  }
  return tz;
}

// IANA 名与中文偏移并排（America/New_York（西四区））；纯偏移串只显示偏移。
// 带上原始时区名，用户才能核对是浏览器设置还是 IP 库的数据不对
function tzLabel(tz: string): string {
  const zone = tzToZone(tz);
  return tz.includes("/") ? `${tz}（${zone}）` : zone;
}

// ---- 身份读数 ----

function buildIdentity(
  a: IdentityAnalysis,
  intel: IPIntel | undefined,
): IdentityReadout {
  const present = !!(intel && intel.ip && intel.country !== "LOCAL");
  // Hosting 不进标签（类型已显示「机房 IP」，重复无意义）；只保留代理类标记
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
    humanRatio: intel?.human_ratio,
    ceilingNote,
    flags,
    fraudScore: a.fraud_score,
    blacklist: a.blacklist_hits ?? [],
    openPorts: a.open_ports ?? [],
    exposure: a.exposure_tags ?? [],
    present,
  };
}

// ---- 四组维度（纯后端评分，前端只映射展示，不再本地计算分数/状态）----

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

// 维度小注：纯展示文本，随后端语义状态派生（不参与评分）
function groupNote(key: ReportGroup["key"], state: SignalState): string {
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

function buildGroups(dims: DimensionScore[]): ReportGroup[] {
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
      note: groupNote(key, state),
    };
  });
}

// ---- 破绽清单 ----

function buildFindings(a: ScanAnalysis, id: IdentityReadout): Finding[] {
  const F: Finding[] = [];
  const push = (f: Finding) => F.push(f);

  // 泄露类（最高优先级）
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
  if (pv.dns.status === "danger") {
    push({
      id: "dns-leak",
      severity: "leak",
      title: "DNS 与出口 IP 地区不一致",
      fact: "DNS 解析地区与出口 IP 地区不一致",
      meaning: "DNS 与出口地区不一致会降低环境一致性。",
      fix: "检查系统或浏览器 DNS 设置。",
    });
  }
  if (pv.ipv6.status === "danger") {
    push({
      id: "ipv6-leak",
      severity: "leak",
      title: "IPv6 与出口 IP 不一致",
      fact: `检测到全局 IPv6 地址${pv.ipv6.address ? `（${pv.ipv6.address}）` : ""}`,
      meaning: "IPv6 与当前出口不一致，可能形成另一条公网出口。",
      fix: "检查 IPv6 路由或使用支持 IPv6 的代理。",
    });
  }

  // 身份类
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

  // 一致性类
  const tz = a.consistency.timezone;
  if (!tz.passed) {
    push({
      id: "tz-mismatch",
      severity: "contradiction",
      title: "时区与 IP 归属地冲突",
      fact: `浏览器时区 ${tzLabel(tz.browser)}，IP 时区 ${tzLabel(tz.expected)}`,
      meaning: "时区对不上是最常见的环境破绽，单项就足以拉低可信度。",
      fix: "把系统时区改成与 IP 归属地一致。",
    });
  }
  const lang = a.consistency.language;
  if (!lang.passed) {
    push({
      id: "lang-mismatch",
      severity: "contradiction",
      title: "系统语言与 IP 国家不匹配",
      fact: `浏览器语言 ${langName(lang.browser)}，IP 国家（${lang.ip_country || "?"}）当地常用 ${langName((lang.expected || [])[0] || "")}`,
      meaning: "语言与地理位置对不上，是环境不一致的直接证据。",
      fix: "把浏览器首选语言调成 IP 归属地对应语言。",
    });
  }
  // 环境指纹类
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
  if (fp.canvas.is_hooked || fp.audio.is_hooked) {
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

// ---- 明细读数 ----

function consistencyRows(a: ScanAnalysis): ReadoutRow[] {
  const tz = a.consistency.timezone;
  const lang = a.consistency.language;
  const tzUnknown = ["unknown", "-", ""].includes(tz.expected);
  return [
    {
      label: "时区",
      value: `${tzToZone(tz.browser)} ↔ ${tzToZone(tz.expected)}`,
      state: tzUnknown ? "unknown" : tz.passed ? "coherent" : "contradiction",
      compare: { browser: tzToZone(tz.browser), ip: tzToZone(tz.expected) },
      hint: tzUnknown ? "IP 时区未知" : undefined,
    },
    {
      label: "语言",
      value: `${langName(lang.browser)} ↔ ${langName((lang.expected || [])[0] || "")}`,
      state: lang.passed ? "coherent" : "contradiction",
      compare: {
        browser: langName(lang.browser),
        ip: langName((lang.expected || [])[0] || ""),
      },
    },
  ];
}

function isIPv6Address(ip: string): boolean {
  return ip.includes(":");
}

function allSameAddress(addrs: string[], target: string): boolean {
  const t = target.toLowerCase();
  return addrs.length > 0 && addrs.every((ip) => ip.toLowerCase() === t);
}

function isExpectedDualStackWebRTC(
  pv: ScanAnalysis["privacy"],
  exitIP = "",
): boolean {
  const extra = pv.webrtc.leaked_ips ?? [];
  if (extra.length !== 1 || !isIPv6Address(extra[0])) return false;
  if (
    pv.ipv6.address &&
    extra[0].toLowerCase() === pv.ipv6.address.toLowerCase()
  )
    return true;
  return !!exitIP && !isIPv6Address(exitIP);
}

function leakRows(a: ScanAnalysis, exitIP = ""): ReadoutRow[] {
  const pv = a.privacy;
  // 仅「danger」算真泄露；warning=仅内网候选、blocked=已挡、disabled=无候选，均非泄露
  const webrtcState: SignalState =
    pv.webrtc.status === "danger"
      ? "leak"
      : pv.webrtc.status === "warning"
        ? "unknown"
        : "coherent";
  const dnsState: SignalState =
    pv.dns.status === "danger"
      ? "leak"
      : pv.dns.status === "blocked"
        ? "unknown"
        : "coherent";
  const ipv6State: SignalState =
    pv.ipv6.status === "danger" ? "leak" : "coherent";
  const webrtcValue = (() => {
    if (pv.webrtc.status === "blocked") return "未检测到公网地址";
    if (pv.webrtc.status === "safe") return "与出口 IP 一致";
    if (pv.webrtc.status === "danger") return "公网地址与出口 IP 不一致";

    const extra = pv.webrtc.leaked_ips ?? [];
    if (extra.length === 0) return "未检测到公网地址";
    if (pv.ipv6.address && allSameAddress(extra, pv.ipv6.address))
      return "检测到 IPv6 双栈";
    if (
      extra.length === 1 &&
      isIPv6Address(extra[0]) &&
      exitIP &&
      !isIPv6Address(exitIP)
    )
      return "检测到 IPv6 双栈";
    return "公网地址与出口 IP 不一致";
  })();

  const ipv6Value = (() => {
    if (pv.ipv6.status === "danger") return "IPv6 与出口 IP 不一致";
    if (!pv.ipv6.address || pv.ipv6.status === "disabled")
      return "未检测到 IPv6";
    if (
      exitIP &&
      isIPv6Address(exitIP) &&
      pv.ipv6.address.toLowerCase() === exitIP.toLowerCase()
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
      value:
        pv.dns.status === "danger"
          ? "与出口 IP 地区不一致"
          : pv.dns.status === "blocked"
            ? "未检测"
            : "与出口 IP 地区一致",
      state: dnsState,
      hint:
        pv.dns.status === "blocked"
          ? undefined
          : pv.dns.matches_exit_ip
            ? "与出口 IP 地区一致"
            : "与出口 IP 不在同一地区",
    },
    {
      label: "IPv6",
      value: ipv6Value,
      state: ipv6State,
    },
  ];
}

function environmentRows(
  a: ScanAnalysis,
  c: AllCollectorResults,
  d: DeviceSignals,
): ReadoutRow[] {
  const fp = a.fingerprint;
  const nav = c.environment.navigator?.data;
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
      state: fp.canvas.is_hooked ? "contradiction" : "coherent",
    },
    {
      label: "Audio 指纹",
      value: fp.audio.is_hooked ? "被篡改" : "正常",
      state: fp.audio.is_hooked ? "contradiction" : "coherent",
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

// ---- RTT 物理延迟读数 ----

// 只透传三个能讲清的延迟值；两侧全缺（或旧缓存无 rtt 字段）时返回 undefined，整条不渲染。
// Δ 可为 0 或负（测量噪声），用存在性判断而非 >0。
function buildRTT(a: ScanAnalysis): RTTReadout | undefined {
  const r = a.rtt;
  if (!r || r.status === "unavailable") return undefined;
  const pos = (x?: number) => (typeof x === "number" && x > 0 ? x : undefined);
  const out: RTTReadout = {
    clientMinMS: pos(r.client_min_ms),
    serverTCPMS: pos(r.server_tcp_ms),
    deltaMS: typeof r.delta_ms === "number" ? r.delta_ms : undefined,
    jitterMS: pos(r.client_jitter_ms),
  };
  if (out.clientMinMS == null && out.serverTCPMS == null) return undefined;
  return out;
}

// ---- 顶层判定 ----

function buildVerdict(groups: ReportGroup[]): ReportModel["verdict"] {
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
    sub: "网络身份与环境特征基本一致。",
  };
}

// ---- 主入口 ----

export function buildReport(
  resp: ScanResponse,
  collectors: AllCollectorResults,
  device: DeviceSignals,
): ReportModel {
  const a = resp.analysis;
  const groups = buildGroups(resp.dimensions ?? []);
  const identity = buildIdentity(a.identity, resp.server_data.ip_intel);
  const idGroup = groups.find((g) => g.key === "identity");
  if (idGroup) identity.state = idGroup.state; // 身份卡状态色与后端评分对齐
  const verdict = buildVerdict(groups);

  return {
    score: resp.score,
    riskLevel: resp.risk_level,
    verdict,
    groups,
    identity,
    findings: buildFindings(a, identity),
    consistency: consistencyRows(a),
    leaks: leakRows(a, identity.ip),
    environment: environmentRows(a, collectors, device),
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

/** 供 CSS 变量映射的颜色键 */
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

/** 状态点柔光背景 */
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
