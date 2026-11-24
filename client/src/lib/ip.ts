/**
 * IPv4 / IPv6 规范化与前缀比较工具（RFC 5952）。
 * 用于避免 IPv6 补零全写（如 240e:0469:...0329）与压缩写法（240e:469:...329）
 * 在字符串比对时误判为不同地址，以及折合同一主机接口的 RFC 4941 /64 临时隐私地址。
 */

function parseIPv4Octets(raw: string): number[] | null {
  const parts = raw.split('.');
  if (parts.length !== 4) return null;
  const out: number[] = [];
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) return null;
    if (part.length > 1 && part.startsWith('0')) return null;
    const n = Number(part);
    if (!Number.isInteger(n) || n < 0 || n > 255) return null;
    out.push(n);
  }
  return out;
}

/** 将 IPv6 文本解析为 8 个 16 位无符号整数（hextets）；非法则返回 null。 */
export function parseIPv6Words(raw: string): number[] | null {
  let s = raw.trim().toLowerCase();
  if (!s || !s.includes(':')) return null;
  const zoneIdx = s.indexOf('%');
  if (zoneIdx !== -1) s = s.slice(0, zoneIdx);

  const doubleColonParts = s.split('::');
  if (doubleColonParts.length > 2) return null;

  const parseSide = (side: string): number[] | null => {
    if (!side) return [];
    const tokens = side.split(':');
    const words: number[] = [];
    for (let i = 0; i < tokens.length; i++) {
      const tok = tokens[i];
      if (!tok) return null;
      if (tok.includes('.')) {
        if (i !== tokens.length - 1) return null;
        const octets = parseIPv4Octets(tok);
        if (!octets) return null;
        words.push((octets[0] << 8) | octets[1], (octets[2] << 8) | octets[3]);
      } else {
        if (!/^[0-9a-f]{1,4}$/.test(tok)) return null;
        words.push(parseInt(tok, 16));
      }
    }
    return words;
  };

  if (doubleColonParts.length === 1) {
    const words = parseSide(doubleColonParts[0]);
    return words && words.length === 8 ? words : null;
  }

  const left = parseSide(doubleColonParts[0]);
  const right = parseSide(doubleColonParts[1]);
  if (!left || !right) return null;
  const missing = 8 - left.length - right.length;
  if (missing < 1) return null;
  return [...left, ...new Array<number>(missing).fill(0), ...right];
}

function isIPv4MappedWords(words: number[]): boolean {
  return (
    words[0] === 0 &&
    words[1] === 0 &&
    words[2] === 0 &&
    words[3] === 0 &&
    words[4] === 0 &&
    words[5] === 0xffff
  );
}

function formatIPv6RFC5952(words: number[]): string {
  if (isIPv4MappedWords(words)) {
    return `${words[6] >> 8}.${words[6] & 0xff}.${words[7] >> 8}.${words[7] & 0xff}`;
  }

  let bestStart = -1;
  let bestLen = 0;
  let curStart = -1;
  let curLen = 0;
  for (let i = 0; i < 8; i++) {
    if (words[i] === 0) {
      if (curStart === -1) curStart = i;
      curLen++;
      if (curLen > bestLen) {
        bestStart = curStart;
        bestLen = curLen;
      }
    } else {
      curStart = -1;
      curLen = 0;
    }
  }

  const hex = words.map((w) => w.toString(16));
  if (bestLen < 2) return hex.join(':');
  const left = hex.slice(0, bestStart).join(':');
  const right = hex.slice(bestStart + bestLen).join(':');
  return `${left}::${right}`;
}

/** 规范化 IPv4 / IPv6 字符串（RFC 5952 压缩小写，并解包 ::ffff:a.b.c.d）。 */
export function canonicalIP(raw: string | null | undefined): string {
  const s = (raw ?? '').trim();
  if (!s) return '';
  const v4 = parseIPv4Octets(s);
  if (v4) return v4.join('.');
  const v6 = parseIPv6Words(s);
  if (v6) return formatIPv6RFC5952(v6);
  return s.toLowerCase();
}

/** 判断规范化后的地址是否为纯 IPv6（不含 ::ffff:a.b.c.d）。 */
export function isIPv6Address(raw: string | null | undefined): boolean {
  const words = parseIPv6Words(raw ?? '');
  return words !== null && !isIPv4MappedWords(words);
}

/** 比较两个 IP 地址在规范化后是否相等。 */
export function sameIP(a: string | null | undefined, b: string | null | undefined): boolean {
  const ca = canonicalIP(a);
  const cb = canonicalIP(b);
  return ca !== '' && ca === cb;
}

/** 判断两个地址是否同属同一 IPv6 /64 前缀（覆盖 RFC 4941 临时隐私地址）。 */
export function sameIPv6Prefix64(
  a: string | null | undefined,
  b: string | null | undefined,
): boolean {
  const wa = parseIPv6Words(a ?? '');
  const wb = parseIPv6Words(b ?? '');
  if (!wa || !wb || isIPv4MappedWords(wa) || isIPv4MappedWords(wb)) return false;
  return wa[0] === wb[0] && wa[1] === wb[1] && wa[2] === wb[2] && wa[3] === wb[3];
}

/** 判断候选地址是否与出口 IP 相同，或同属同一 IPv6 /64 前缀。 */
export function sameExitOrPrefix64(
  candidateIP: string | null | undefined,
  exitIP: string | null | undefined,
): boolean {
  return sameIP(candidateIP, exitIP) || sameIPv6Prefix64(candidateIP, exitIP);
}
