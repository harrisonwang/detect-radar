import type { RTTData } from './collectors/types';

export type ScanScenario = 'strict' | 'daily';
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

export type DetectStatus = 'safe' | 'warning' | 'danger' | 'loading';

export interface NetworkIdentity {
  ip: string;
  country: string;
  country_name?: string;
  city?: string;
  region?: string;
  timezone?: string;
  latitude?: number;
  longitude?: number;
  asn: string;
  org?: string;
  isp?: string;
  fraud_score?: number;
  usage_type: string;
  usage_type_raw?: string;
  confidence?: number;
  detect_method?: string;
  human_ratio?: number;
  tip?: string;
  source?: string;
  tier?: string;
  fetched_at?: string;
  is_vpn?: boolean;
  is_proxy?: boolean;
  is_tor?: boolean;
  is_relay?: boolean;
  is_hosting?: boolean;
  is_mobile?: boolean;
  is_anonymous?: boolean;
}

export interface ConsistencyData {
  timezone: {
    match: boolean;
    value: string;
  };
  language: {
    match: boolean;
    value: string;
  };
}

export interface NetworkPrivacy {
  webrtc: {
    value: string;
    status: DetectStatus;
  };
  dns: {
    value: string;
    status: DetectStatus;
  };
  ipv6: {
    value: string;
    status: DetectStatus;
  };
  tcpws: {
    value: string;
    status: DetectStatus;
    latencyDiff?: number;
  };
}

export interface DeepFingerprint {
  tcpip: {
    os: string;
    match: boolean;
    mismatchInfo?: string;
  };
  canvas: {
    value: string;
    status: DetectStatus;
  };
  audio: {
    value: string;
    status: DetectStatus;
  };
  webgl: {
    value: string;
    status: DetectStatus;
  };
}

export interface DeepDetection {
  isProxy: boolean;
  proxyType?: string;
  fraudScore: number;
  isBlacklisted: boolean;
  blacklistSource?: string;
}

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

export type IPIntel = NetworkIdentity;

export interface CollectorResult<T = unknown> {
  status: 'success' | 'error' | 'unsupported' | 'denied';
  data?: T;
  error?: string;
  timing?: number;
}

export interface FPJSAudit {
  visitorId?: string;
  canvasSummary?: string;
  hook: FingerprintHookAudit;
}

export interface ScanRequest {
  scan_id: string;
  timestamp: number;
  scenario: ScanScenario | ScenarioId;
  fingerprint: Record<string, unknown>;
  environment: Record<string, unknown>;
  leaks: Record<string, unknown>;
  fpjs_audit?: FPJSAudit;
  network?: { rtt?: CollectorResult<RTTData> };
}

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
  dns: { status: 'safe' | 'warning' | 'danger' | 'unknown' | 'blocked'; matches_exit_ip: boolean; country?: string; isp?: string; assessment?: DNSVerdict };
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
  rtt?: RTTAnalysis;
}

export interface RTTAnalysis {
  status: 'ok' | 'partial' | 'unavailable';
  client_min_ms?: number;
  client_median_ms?: number;
  client_jitter_ms?: number;
  server_tcp_ms?: number;
  server_tcp_min_ms?: number;
  server_rtt_var_ms?: number;
  delta_ms?: number;
  delta_min_ms?: number;
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

export interface DimensionScore {
  key: 'identity' | 'consistency' | 'leak' | 'environment';
  label: string;
  score: number;
  state: 'coherent' | 'contradiction' | 'leak' | 'unknown';
  passed: number;
  total: number;
}

export interface ScanResponse {
  scan_id: string;
  status: string;
  scenario?: ScanScenario;
  score: number;
  risk_level: RiskLevel;
  diagnosis: DiagnosisItem[];
  dimensions: DimensionScore[];
  analysis: ScanAnalysis;
  server_data: ScanServerData;
  processed_at: string;
  processing_time_ms: number;
}

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
  asn?: string;
  network_type?: 'public' | 'access' | 'hosting';
  queried_at: string;
}

export interface DNSLeakResult {
  id: string;
  leaked: boolean;
  level: 'safe' | 'warning' | 'danger' | 'unknown';
  dns_servers: DNSQuery[];
  expected_country: string;
  actual_countries: string[];
  recommendation: string;
  assessment?: DNSVerdict;
}

export interface DNSVerdict {
  policy_version?: number;
  state: 'suspected' | 'clear' | 'unknown' | 'unobserved';
  reason: string;
  networks?: string[];
  suspect_ips?: string[];
}

export interface DNSExit {
  ip: string;
  country: string;
  network: string;
  asn?: string;
  type?: 'proxy' | 'hosting' | 'access';
}

export type DNSResolver = Omit<DNSQuery, 'queried_at'> & { rounds: number; round_ids?: number[] };

export interface DNSAssessmentInput {
  exit: DNSExit | null;
  resolvers: DNSResolver[];
  observed_rounds: number;
  total_rounds: number;
}

export interface DNSAssessmentResult {
  verdict: DNSVerdict;
  exit: DNSExit | null;
  resolvers: DNSResolver[];
}

export type WebRTCCandidateRelation =
  | 'matches-exit'
  | 'same-prefix'
  | 'dual-stack'
  | 'same-network'
  | 'leak';

export interface WebRTCNetworkInfo {
  ip: string;
  country?: string;
  network?: string;
  asn?: string;
  type?: 'proxy' | 'hosting' | 'access';
  relation?: WebRTCCandidateRelation;
}

export interface WebRTCAssessmentResponse {
  policy_version: number;
  verdict: {
    policy_version: number;
    state: 'clear' | 'leaked' | 'unknown';
    reason: string;
    suspect_ips: string[];
  };
  exit?: WebRTCNetworkInfo;
  candidates: WebRTCNetworkInfo[];
}

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

export type FingerprintStatus = 'native' | 'protected' | 'conflict' | 'unknown';

export interface FingerprintFeatures {
  overallStatus: FingerprintStatus;
  canvas: {
    hash: string;
    status: 'native' | 'noise' | 'blocked';
    isStable: boolean;
  };
  webgl: {
    vendor: string;
    renderer: string;
    status: 'match' | 'mismatch' | 'unknown';
    mismatchReason?: string;
  };
  audio: {
    hash: string;
    status: 'native' | 'noise' | 'blocked';
  };
  screen: {
    resolution: string;
    viewport: string;
    colorDepth: number;
    status: 'normal' | 'fixed' | 'suspicious';
    hasWindowChrome: boolean;
  };
}

export interface IPReputationData {
  ipType: 'residential' | 'datacenter' | 'commercial' | 'mobile' | 'unknown';
  ipTypeLabel: string;
  ipTypeQuality: 'excellent' | 'good' | 'warning' | 'danger';
  fraudScore: number;
  fraudLevel: 'safe' | 'low' | 'medium' | 'high';
  blacklistStatus: 'clean' | 'hit';
  blacklistSources?: string[];
}

export interface HardwareLogicData {
  cpuCores: number;
  memory: number;
  gpuVendor: string;
  gpuRenderer: string;
  os: string;
  isLogical: boolean;
  issues?: string[];
}

export interface MobileEmulationData {
  touchPoints: number;
  touchStatus: 'normal' | 'suspicious' | 'missing';
  orientation: 'portrait' | 'landscape';
  gyroscope: 'real' | 'emulated' | 'missing';
  battery: { supported: boolean; level?: number; charging?: boolean };
  screenSize: string;
}

export interface BotDetectionData {
  webdriver: boolean;
  cdcProperty: boolean;
  headlessFeatures: boolean;
  automationTraces: string[];
  overallStatus: 'safe' | 'suspicious' | 'detected';
}

export interface TLSFingerprintData {
  ja3Hash?: string;
  ja3Match: 'chrome' | 'firefox' | 'safari' | 'custom' | 'unknown';
  httpVersion: 'HTTP/1.1' | 'HTTP/2' | 'HTTP/3';
  quicSupported: boolean;
}

export interface CheckGroup {
  passed: number;
  total: number;
  status: 'pass' | 'warn' | 'fail';
  subtitle?: string;
}

export interface HealthStatus {
  score: number;
  level: 'ready' | 'warning' | 'fatal';
  summary: string;
  checks: {
    privacyLeak: CheckGroup;
    ipQuality: CheckGroup;
    consistency: CheckGroup;
  };
}

export interface ActionableIssue {
  id: string;
  severity: 'fatal' | 'warning' | 'info';
  title: string;
  problem: string;
  consequence: string;
  fix: string;
}

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

export interface IPQualityData {
  ipType: string;
  ipTypeLabel: string;
  ipTypeQuality: 'excellent' | 'good' | 'warning' | 'danger';
  fraudScore: number;
  fraudLevel: 'safe' | 'low' | 'medium' | 'high';
  blacklistStatus: 'clean' | 'hit';
  blacklistSources?: string[];
}

export interface EnvironmentConsistencyData {
  timezoneMatch: boolean;
  timezoneOffset: number;
  languageMatch: boolean;
  systemLanguage: string;
  ipCountryLanguage: string;
  cpuCores: number;
  memory: number;
  gpuVendor: string;
  gpuRenderer: string;
  os: string;
  isLogical: boolean;
  hardwareIssues?: string[];
  canvasStatus: 'native' | 'noise' | 'blocked';
  webglMatch: boolean;
  webglMismatchDetail?: string;
  audioStatus: 'native' | 'noise' | 'blocked';
}
