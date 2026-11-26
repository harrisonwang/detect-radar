

export interface CollectorResult<T = unknown> {
  status: 'success' | 'error' | 'unsupported' | 'denied';
  data?: T;
  error?: string;
  timing?: number;
}

export interface CanvasFingerprintData {
  hash_sample_a: string;
  hash_sample_b: string;
  native_code: string;
  data_length: number;
}

export interface AudioFingerprintData {
  hash_sample: string;
  entropy_raw: number;
  native_code: string;
}

export interface WebGLCapabilities {
  max_texture_size: number;
  max_anisotropy: number;
}

export interface WebGLImageFingerprint {
  hash: string;
  pixels_length: number;
}

export interface WebGLFingerprintData {
  context_name: string;
  base_vendor: string | null;
  base_renderer: string | null;
  unmasked_vendor: string | null;
  unmasked_renderer: string | null;
  extensions_hash: string;
  extensions_count: number;
  capabilities: WebGLCapabilities;
  image: WebGLImageFingerprint;
  native_code: string;
}

export interface FingerprintResults {
  canvas?: CollectorResult<CanvasFingerprintData>;
  audio?: CollectorResult<AudioFingerprintData>;
  webgl?: CollectorResult<WebGLFingerprintData>;
}

export interface NavigatorData {
  userAgent: string;
  platform: string;
  language: string;
  languages: string[];
  hardwareConcurrency: number;
  deviceMemory: number | null;
  maxTouchPoints: number;
  cookieEnabled: boolean;
  doNotTrack: string | null;
  webdriver: boolean | undefined;
  vendor: string;
  plugins_length: number;
}

export interface UADataBrand {
  brand: string;
  version: string;
}

export interface UserAgentDataInfo {
  brands: UADataBrand[];
  mobile: boolean;
  platform: string;
  architecture?: string;
  bitness?: string;
  model?: string;
  platformVersion?: string;
  fullVersionList?: UADataBrand[];
}

export interface ScreenData {
  width: number;
  height: number;
  availWidth: number;
  availHeight: number;
  colorDepth: number;
  pixelDepth: number;
  devicePixelRatio: number;
}

export interface TimezoneData {
  timezone: string;
  timezoneOffset: number;
}

export interface EnvironmentResults {
  navigator?: CollectorResult<NavigatorData>;
  userAgentData?: CollectorResult<UserAgentDataInfo>;
  screen?: CollectorResult<ScreenData>;
  timezone?: CollectorResult<TimezoneData>;
}

export interface WebRTCLeakData {
  leaked: boolean;
  local_ips: string[];
  public_ips: string[];
  stun_ips: string[];
}

export interface DNSLeakData {
  leaked: boolean;
  assessed?: boolean;
  test_id?: string;
  resolvers: string[];
}

export interface IPv6LeakData {
  leaked: boolean;
  ipv6_address: string | null;
}

export interface LeaksResults {
  webrtc?: CollectorResult<WebRTCLeakData>;
  dns?: CollectorResult<DNSLeakData>;
  ipv6?: CollectorResult<IPv6LeakData>;
}

export interface RTTData {
  probe_id?: string;
  samples: number[];
  min_ms: number;
  median_ms: number;
  jitter_ms: number;
  count: number;
  connect_ms?: number;
  conn_reused?: boolean;
}

export interface NetworkResults {
  rtt?: CollectorResult<RTTData>;
}

export interface AllCollectorResults {
  fingerprint: FingerprintResults;
  environment: EnvironmentResults;
  leaks: LeaksResults;
  network?: NetworkResults;
}
