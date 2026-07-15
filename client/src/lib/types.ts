import type { RTTData } from './collectors/types';

// Scenario types
export type ScenarioId = 'general' | 'ecommerce' | 'social';

export interface DiagnosisItem {
  status: 'success' | 'warning' | 'error';
  text: string;
}

export interface ScenarioData {
  id: ScenarioId;
  label: string;
  score: number;
  color: string;
  badge: string;
  analysis: DiagnosisItem[];
  showAd: boolean;
  adText?: string;
}

// Detection item status
export type DetectStatus = 'safe' | 'warning' | 'danger' | 'loading';

// 网络身份（IP 信息）
export interface NetworkIdentity {
  ip: string;
  country: string;               // ISO 3166-1 alpha-2
  country_name?: string;         // 国家全称
  city?: string;                 // 城市
  region?: string;               // 省/州
  timezone?: string;             // IANA 时区
  latitude?: number;
  longitude?: number;
  asn: string;                   // AS4134
  org?: string;                  // 组织名称
  isp?: string;                  // ISP 名称（比 org 更具体）
  fraud_score?: number;          // 欺诈风险评分 (0-100)
  usage_type: string;            // isp/hosting/unknown
  usage_type_raw?: string;       // 原始类型
  confidence?: number;           // 置信度 0-100
  detect_method?: string;        // 检测方法
  human_ratio?: number;          // 真实用户占比 (0-100)
  tip?: string;                  // 提示文案
  source?: string;               // 数据来源
  tier?: string;                 // 查询层级
  fetched_at?: string;           // 获取时间 (ISO)
  is_vpn?: boolean;
  is_proxy?: boolean;
  is_tor?: boolean;
  is_relay?: boolean;
  is_hosting?: boolean;
  is_mobile?: boolean;
  is_anonymous?: boolean;
}

// 环境一致性
export interface ConsistencyData {
  timezone: {
    match: boolean;
    value: string;           // Asia/Shanghai
  };
  language: {
    match: boolean;
    value: string;           // zh-CN
  };
}

// 网络隐私
export interface NetworkPrivacy {
  webrtc: {
    value: string;           // 安全（已屏蔽）、安全、存在泄漏
    status: DetectStatus;
  };
  dns: {
    value: string;           // 安全、存在泄漏
    status: DetectStatus;
  };
  ipv6: {
    value: string;           // 未检测到、安全、存在泄漏
    status: DetectStatus;
  };
  tcpws: {
    value: string;           // 正常、延迟异常（疑似代理）
    status: DetectStatus;
    latencyDiff?: number;    // 延迟差异 (ms)
  };
}

// 底层指纹
export interface DeepFingerprint {
  tcpip: {
    os: string;              // Windows、Linux
    match: boolean;          // 一致、不一致
    mismatchInfo?: string;   // 不一致时的详情，如 (UA: Win)
  };
  canvas: {
    value: string;           // 原生 (Native)、检测到噪音 (Noise)
    status: DetectStatus;
  };
  audio: {
    value: string;           // 原生 (Native)、检测到噪音 (Noise)
    status: DetectStatus;
  };
  webgl: {
    value: string;           // 原生 (Native)、检测到噪音 (Noise)
    status: DetectStatus;
  };
}

// 深度检测 (IP 相关，需调用第三方 API)
export interface DeepDetection {
  isProxy: boolean;          // 是否代理
  proxyType?: string;        // 代理类型: VPN, Tor, Residential Proxy 等
  fraudScore: number;        // 欺诈评分 0-100
  isBlacklisted: boolean;    // 是否在黑名单
  blacklistSource?: string;  // 黑名单来源
}

// 指纹审计相关类型
export interface FingerprintHookAudit {
  status: 'SAFE' | 'DANGER';
  message: string;
  methods: string[];
}

export interface FingerprintAuditPayload {
  visitorId?: string;
  canvasSummary?: string;
  hook: FingerprintHookAudit;
}

// ============================================================================
// 后端 API 契约（与 server/internal/model 的 json tag 逐字对齐）
// ============================================================================

/** IP 信息（后端 model.IPIntel）——等价于 NetworkIdentity，供 API 层使用 */
export type IPIntel = NetworkIdentity;

// ---- POST /scans 请求 ----

/** 采集器通用包装（对齐 model.CollectorResult） */
export interface CollectorResult<T = unknown> {
  status: 'success' | 'error' | 'unsupported' | 'denied';
  data?: T;
  error?: string;
  timing?: number;
}

/** FingerprintJS Hook 审计（对齐 model.FPJSAudit） */
export interface FPJSAudit {
  visitorId?: string;
  canvasSummary?: string; // 后端忽略，仅调试
  hook: FingerprintHookAudit;
}

/**
 * POST /scans 请求体。fingerprint/environment/leaks 三段直接复用 collectors 的输出
 * （字段名与后端 json tag 完全一致，无需转换）。此处用宽松类型承接，避免与
 * collectors/types.ts 的具体结构耦合。
 */
export interface ScanRequest {
  scan_id: string;
  timestamp: number;
  scenario: ScenarioId;
  fingerprint: Record<string, unknown>;
  environment: Record<string, unknown>;
  leaks: Record<string, unknown>;
  fpjs_audit?: FPJSAudit;
  // RTT 探测（Phase 1 仅采集上报，后端影子模式标定，不参与前端评分）
  network?: { rtt?: CollectorResult<RTTData> };
}

// ---- POST /scans 响应（对齐 model.ScanResponse / ScanAnalysis） ----

export type RiskLevel = 'safe' | 'low' | 'medium' | 'high' | 'critical';

export interface IdentityAnalysis {
  ip_type: 'residential' | 'datacenter' | 'mobile' | 'unknown';
  ip_type_confidence: number;
  verdict: 'residential_pass' | 'datacenter_fail' | 'gray_unknown';
  fraud_score: number;
  is_proxy: boolean;
  is_vpn: boolean;
  is_tor: boolean;
  is_hosting: boolean;
  human_ratio?: number;
  blacklist_hits: string[];
  open_ports?: number[];
  open_proxy_port?: boolean;
  is_tor_exit?: boolean;
  exposure_tags?: string[];
}

export interface FingerprintAnalysis {
  canvas: { status: 'native' | 'noise' | 'blocked'; is_stable: boolean; is_hooked: boolean };
  audio: { status: 'native' | 'blocked' | 'unknown'; is_hooked: boolean };
  webgl: {
    status: 'match' | 'mismatch' | 'unknown';
    mismatch_reason?: string;
    vendor_matches_ua: boolean;
  };
  overall_status: 'native' | 'protected' | 'conflict' | 'unknown';
}

export interface PrivacyAnalysis {
  webrtc: {
    status: 'safe' | 'warning' | 'danger' | 'blocked';
    leaked_ips: string[];
    matches_exit_ip: boolean;
  };
  dns: { status: 'safe' | 'danger' | 'blocked'; matches_exit_ip: boolean };
  ipv6: { status: 'safe' | 'danger' | 'disabled'; address?: string };
  overall_status: 'safe' | 'warning' | 'danger' | 'disabled';
}

export interface TimezoneCheck {
  passed: boolean;
  browser: string;
  expected: string;
  offset_hours?: number;
}

export interface LanguageCheck {
  passed: boolean;
  browser: string;
  expected: string[];
  ip_country: string;
}

export interface ConsistencyChecks {
  timezone: TimezoneCheck;
  language: LanguageCheck;
}

export interface AutomationAnalysis {
  webdriver_detected: boolean;
  automation_traces: string[];
  overall_status: 'safe' | 'suspicious' | 'detected';
}

export interface ScanAnalysis {
  identity: IdentityAnalysis;
  fingerprint: FingerprintAnalysis;
  privacy: PrivacyAnalysis;
  consistency: ConsistencyChecks;
  automation: AutomationAnalysis;
  // 可选：GET /scans/:id 可能取回旧后端缓存的扫描（无 rtt 字段）
  rtt?: RTTAnalysis;
}

/** RTT 物理探测派生视图（Phase 1 只呈现读数不计分；与 model.RTTAnalysis 的 json tag 逐字对齐） */
export interface RTTAnalysis {
  status: 'ok' | 'partial' | 'unavailable';
  client_min_ms?: number;    // 浏览器↔服务器全程（含代理腿），多次采样取最小
  client_median_ms?: number;
  client_jitter_ms?: number;
  server_tcp_ms?: number;    // nginx 内核实测：出口 IP↔服务器
  server_rtt_var_ms?: number;
  delta_ms?: number;         // client_min - server_tcp ≈ 浏览器↔出口 这一腿
  geo_min_ms?: number;
  geo_distance_km?: number;
  geo_violation?: boolean;
  geo_source?: string;
  connect_ms?: number;
  conn_reused?: boolean;
}

export interface ScanServerData {
  client_ip: string;
  ip_intel?: IPIntel;
  ja3_hash?: string;
  http_version?: string;
}

// 四轴雷达分（纯后端评分，前端直接消费，不再本地计算）
export interface DimensionScore {
  key: 'identity' | 'consistency' | 'leak' | 'environment';
  label: string;
  score: number; // 0-100
  state: 'coherent' | 'contradiction' | 'leak' | 'unknown';
  passed: number;
  total: number;
}

export interface ScanResponse {
  scan_id: string;
  status: string;
  score: number;
  risk_level: RiskLevel;
  diagnosis: DiagnosisItem[];
  dimensions: DimensionScore[];
  analysis: ScanAnalysis;
  server_data: ScanServerData;
  processed_at: string;
  processing_time_ms: number;
}

// ---- DNS 泄露端点（POST /leaks/dns, GET /leaks/dns/:id） ----

export interface DNSLeakTestResponse {
  id: string;
  test_domain: string;
  created_at: string;
  expires_at: string;
}

export interface DNSQuery {
  ip: string;
  country: string;
  isp: string;
  queried_at: string;
}

export interface DNSLeakResult {
  id: string;
  leaked: boolean;
  level: 'safe' | 'warning' | 'danger';
  dns_servers: DNSQuery[];
  expected_country: string;
  actual_countries: string[];
  recommendation: string;
}

// ============ 场景专属卡片类型 ============

// 网络隐私泄露检测（重构版）
export type PrivacyLeakStatus = 'safe' | 'blocked' | 'warning' | 'danger' | 'disabled';

export interface WebRTCLeakData {
  status: PrivacyLeakStatus;
  publicIP?: string;
  localIP?: string;
  matchesExitIP: boolean;
}

export interface DNSLeakData {
  status: PrivacyLeakStatus;
  location?: string;
  matchesExitIP: boolean;
  serverCount?: number;
  provider?: string;
  suggestion?: string;
}

export interface IPv6LeakData {
  status: PrivacyLeakStatus;
  address?: string;
  matchesExitIPv4?: boolean;
  location?: string;
  suggestion?: string;
}

export interface PrivacyLeakDetection {
  overallStatus: PrivacyLeakStatus;
  webrtc: WebRTCLeakData;
  dns: DNSLeakData;
  ipv6: IPv6LeakData;
}

// 通用场景 - 浏览器指纹特征
export type FingerprintStatus = 'native' | 'protected' | 'conflict' | 'unknown';

export interface FingerprintFeatures {
  overallStatus: FingerprintStatus;
  canvas: {
    hash: string;
    status: 'native' | 'noise' | 'blocked';
    isStable: boolean; // 多次读取是否一致
  };
  webgl: {
    vendor: string;
    renderer: string;
    status: 'match' | 'mismatch' | 'unknown';
    mismatchReason?: string; // 冲突原因，如 "与 Windows 系统不符"
  };
  audio: {
    hash: string;
    status: 'native' | 'noise' | 'blocked';
  };
  screen: {
    resolution: string; // 如 "1920 x 1080"
    viewport: string;   // 如 "1920 x 1040"
    colorDepth: number;
    status: 'normal' | 'fixed' | 'suspicious';
    hasWindowChrome: boolean; // 是否有任务栏/边框空间
  };
}


// 跨境电商 - IP 欺诈与信誉
export interface IPReputationData {
  ipType: 'residential' | 'datacenter' | 'commercial' | 'mobile' | 'unknown';
  ipTypeLabel: string;
  ipTypeQuality: 'excellent' | 'good' | 'warning' | 'danger';
  fraudScore: number;
  fraudLevel: 'safe' | 'low' | 'medium' | 'high';
  blacklistStatus: 'clean' | 'hit';
  blacklistSources?: string[];
}

// 跨境电商 - 硬件逻辑一致性
export interface HardwareLogicData {
  cpuCores: number;
  memory: number; // GB
  gpuVendor: string;
  gpuRenderer: string;
  os: string;
  isLogical: boolean;
  issues?: string[];
}

// 社交媒体 - 移动环境模拟
export interface MobileEmulationData {
  touchPoints: number;
  touchStatus: 'normal' | 'suspicious' | 'missing';
  orientation: 'portrait' | 'landscape';
  gyroscope: 'real' | 'emulated' | 'missing';
  battery: { supported: boolean; level?: number; charging?: boolean };
  screenSize: string;
}

// 社交媒体 - 自动化与机器人检测
export interface BotDetectionData {
  webdriver: boolean;
  cdcProperty: boolean;
  headlessFeatures: boolean;
  automationTraces: string[];
  overallStatus: 'safe' | 'suspicious' | 'detected';
}

// 社交媒体 - TLS 指纹与协议
export interface TLSFingerprintData {
  ja3Hash?: string;
  ja3Match: 'chrome' | 'firefox' | 'safari' | 'custom' | 'unknown';
  httpVersion: 'HTTP/1.1' | 'HTTP/2' | 'HTTP/3';
  quicSupported: boolean;
}

// ============ 跨境电商 V4.0 - 三层信息架构 ============

// 健康状态检查组
export interface CheckGroup {
  passed: number;
  total: number;
  status: 'pass' | 'warn' | 'fail';
  subtitle?: string;
}

// L1 - 健康状态
export interface HealthStatus {
  score: number;
  level: 'ready' | 'warning' | 'fatal';
  summary: string;
  checks: {
    privacyLeak: CheckGroup;    // 隐私泄露检测
    ipQuality: CheckGroup;      // IP 质量验证
    consistency: CheckGroup;    // 环境一致性
  };
}

// L2 - 问题诊断
export interface ActionableIssue {
  id: string;
  severity: 'fatal' | 'warning' | 'info';
  title: string;
  problem: string;           // 检测到了什么（事实）
  consequence: string;       // 这意味着什么（风险）
  fix: string;               // 怎么解决
}

// L3 - 隐私泄露检测
export interface PrivacyLeakCheckData {
  webrtc: {
    status: 'safe' | 'blocked' | 'danger';
    leakedIP?: string;
  };
  dns: {
    status: 'safe' | 'warning' | 'danger';
    location: string;
    matchesExitIP: boolean;
  };
  ipv6: {
    status: 'safe' | 'disabled' | 'danger';
    address?: string;
  };
}

// L3 - IP 质量验证
export interface IPQualityData {
  ipType: string;
  ipTypeLabel: string;
  ipTypeQuality: 'excellent' | 'good' | 'warning' | 'danger';
  fraudScore: number;
  fraudLevel: 'safe' | 'low' | 'medium' | 'high';
  blacklistStatus: 'clean' | 'hit';
  blacklistSources?: string[];
}

// L3 - 环境一致性
export interface EnvironmentConsistencyData {
  // 时区/语言一致性
  timezoneMatch: boolean;
  timezoneOffset: number;
  languageMatch: boolean;
  systemLanguage: string;
  ipCountryLanguage: string;
  // 硬件配置
  cpuCores: number;
  memory: number;
  gpuVendor: string;
  gpuRenderer: string;
  os: string;
  isLogical: boolean;
  hardwareIssues?: string[];
  // 指纹拟人度
  canvasStatus: 'native' | 'noise' | 'blocked';
  webglMatch: boolean;
  webglMismatchDetail?: string; // 如 "UA 声明 iPhone，但 GPU 为 NVIDIA 3080"
  audioStatus: 'native' | 'noise' | 'blocked';
}
