

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
}

export interface AudioFingerprintData {
  native_code: string;
}

export interface WebGLFingerprintData {
  unmasked_renderer: string | null;
}

export interface FingerprintResults {
  canvas?: CollectorResult<CanvasFingerprintData>;
  audio?: CollectorResult<AudioFingerprintData>;
  webgl?: CollectorResult<WebGLFingerprintData>;
}

export interface NavigatorData {
  platform: string;
  language: string;
  languages: string[];
  hardwareConcurrency: number;
  deviceMemory: number | null;
  maxTouchPoints: number;
  webdriver: boolean | undefined;
  plugins_length: number;
}

export interface UserAgentDataInfo {
  mobile: boolean;
}

export interface TimezoneData {
  timezone: string;
}

export interface EnvironmentResults {
  navigator?: CollectorResult<NavigatorData>;
  userAgentData?: CollectorResult<UserAgentDataInfo>;
  timezone?: CollectorResult<TimezoneData>;
}

export interface WebRTCLeakData {
  local_ips: string[];
  public_ips: string[];
}

export interface DNSLeakData {
  test_id?: string;
  resolvers: string[];
}

export interface IPv6LeakData {
  leaked: boolean;
  ipv6_address: string | null;
  ipv4_address?: string | null;
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
