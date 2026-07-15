/**
 * 采集器类型定义
 * 按功能域分组：Fingerprint / Environment / Leaks
 */

/* ========================================
 * 通用类型
 * ======================================== */

/** 采集器通用返回结构 */
export interface CollectorResult<T = unknown> {
  status: 'success' | 'error' | 'unsupported' | 'denied';
  data?: T;
  error?: string;
  timing?: number; // 采集耗时 (ms)
}

/* ========================================
 * 指纹类 (Fingerprint)
 * 硬件/渲染层面的底层特征
 * ======================================== */

/** Canvas 指纹数据 */
export interface CanvasFingerprintData {
  hash_sample_a: string;  // 样本 A 的哈希值
  hash_sample_b: string;  // 样本 B 的哈希值 (用于稳定性检测)
  native_code: string;    // toDataURL 原生代码字符串
  data_length: number;    // 原始数据长度
}

/** Audio 指纹数据 */
export interface AudioFingerprintData {
  hash_sample: string;    // 样本哈希值
  entropy_raw: number;    // 原始熵值
  native_code: string;    // OfflineAudioContext 原生代码字符串
}

/** WebGL 能力参数 */
export interface WebGLCapabilities {
  max_texture_size: number;
  max_anisotropy: number;
}

/** WebGL 图像指纹 */
export interface WebGLImageFingerprint {
  hash: string;           // 渲染图像哈希
  pixels_length: number;  // 像素数据长度
}

/** WebGL 指纹数据 */
export interface WebGLFingerprintData {
  // Metadata
  context_name: string;
  base_vendor: string | null;
  base_renderer: string | null;
  unmasked_vendor: string | null;
  unmasked_renderer: string | null;
  extensions_hash: string;
  extensions_count: number;
  capabilities: WebGLCapabilities;
  // Image
  image: WebGLImageFingerprint;
  // Native code
  native_code: string;
}

/** 指纹采集结果 */
export interface FingerprintResults {
  canvas?: CollectorResult<CanvasFingerprintData>;
  audio?: CollectorResult<AudioFingerprintData>;
  webgl?: CollectorResult<WebGLFingerprintData>;
}

/* ========================================
 * 环境类 (Environment)
 * 浏览器/系统环境信息
 * ======================================== */

/** Navigator 数据 */
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
  // 新增：用于检测
  webdriver: boolean | undefined;   // 🔴 红线：自动化浏览器标记
  vendor: string;                   // 🟡 浏览器供应商
  plugins_length: number;           // 🟡 iOS 应为 0
}

/** UserAgentData Brand 信息 */
export interface UADataBrand {
  brand: string;
  version: string;
}

/** UserAgentData 数据 (现代浏览器 API) */
export interface UserAgentDataInfo {
  brands: UADataBrand[];
  mobile: boolean;
  platform: string;
  // 高熵值 (需要异步获取)
  architecture?: string;      // x86, arm
  bitness?: string;           // 64, 32
  model?: string;             // 设备型号
  platformVersion?: string;   // 平台版本
  fullVersionList?: UADataBrand[];
}

/** Screen 数据 */
export interface ScreenData {
  width: number;
  height: number;
  availWidth: number;
  availHeight: number;
  colorDepth: number;
  pixelDepth: number;
  devicePixelRatio: number;
}

/** Timezone 数据 */
export interface TimezoneData {
  timezone: string;         // Asia/Shanghai
  timezoneOffset: number;   // -480 (分钟)
}

/** 环境采集结果 */
export interface EnvironmentResults {
  navigator?: CollectorResult<NavigatorData>;
  userAgentData?: CollectorResult<UserAgentDataInfo>;
  screen?: CollectorResult<ScreenData>;
  timezone?: CollectorResult<TimezoneData>;
}

/* ========================================
 * 泄漏类 (Leaks)
 * 隐私泄漏检测
 * ======================================== */

/** WebRTC 泄漏数据 */
export interface WebRTCLeakData {
  leaked: boolean;
  local_ips: string[];    // 内网 IP
  public_ips: string[];   // 公网 IP (泄漏)
  stun_ips: string[];     // STUN 返回的 IP
}

/** DNS 泄漏数据 */
export interface DNSLeakData {
  leaked: boolean;
  test_id?: string;       // 后端一次性测试 ID
  resolvers: string[];    // DNS 解析器列表
}

/** IPv6 泄漏数据 */
export interface IPv6LeakData {
  leaked: boolean;
  ipv6_address: string | null;
}

/** 泄漏检测结果 */
export interface LeaksResults {
  webrtc?: CollectorResult<WebRTCLeakData>;
  dns?: CollectorResult<DNSLeakData>;
  ipv6?: CollectorResult<IPv6LeakData>;
}

/* ========================================
 * 网络类 (Network)
 * 应用层往返时延等物理网络特征，用于以时延反推代理
 * ======================================== */

/** RTT 探测数据（应用层往返时延，单位 ms） */
export interface RTTData {
  samples: number[];      // 每次探测的应用层往返 ms（已丢弃首次预热）
  min_ms: number;         // 最小值——最承重的统计量（详见 network/rtt.ts）
  median_ms: number;
  jitter_ms: number;      // max - min
  count: number;          // 参与统计的有效样本数
  connect_ms?: number;    // Resource Timing connectEnd-connectStart；H2 复用时不可得
  conn_reused?: boolean;  // true=连接被复用（connect 段为空），false=本次探测新建了连接
}

/** 网络采集结果 */
export interface NetworkResults {
  rtt?: CollectorResult<RTTData>;
}

/* ========================================
 * 聚合结果
 * ======================================== */

/** 所有采集结果 */
export interface AllCollectorResults {
  fingerprint: FingerprintResults;
  environment: EnvironmentResults;
  leaks: LeaksResults;
  network?: NetworkResults;
}
