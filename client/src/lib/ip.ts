
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

export function canonicalIP(raw: string | null | undefined): string {
  const s = (raw ?? '').trim();
  if (!s) return '';
  const v4 = parseIPv4Octets(s);
  if (v4) return v4.join('.');
  const v6 = parseIPv6Words(s);
  if (v6) return formatIPv6RFC5952(v6);
  return s.toLowerCase();
}

export function isIPv6Address(raw: string | null | undefined): boolean {
  const words = parseIPv6Words(raw ?? '');
  return words !== null && !isIPv4MappedWords(words);
}

export function sameIP(a: string | null | undefined, b: string | null | undefined): boolean {
  const ca = canonicalIP(a);
  const cb = canonicalIP(b);
  return ca !== '' && ca === cb;
}

export function sameIPv6Prefix64(
  a: string | null | undefined,
  b: string | null | undefined,
): boolean {
  const wa = parseIPv6Words(a ?? '');
  const wb = parseIPv6Words(b ?? '');
  if (!wa || !wb || isIPv4MappedWords(wa) || isIPv4MappedWords(wb)) return false;
  return wa[0] === wb[0] && wa[1] === wb[1] && wa[2] === wb[2] && wa[3] === wb[3];
}

export function sameExitOrPrefix64(
  candidateIP: string | null | undefined,
  exitIP: string | null | undefined,
): boolean {
  return sameIP(candidateIP, exitIP) || sameIPv6Prefix64(candidateIP, exitIP);
}
