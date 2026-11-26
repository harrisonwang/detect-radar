import { canonicalIP, parseIPv6Words } from './ip';

type MaskStyle = 'dots' | 'star';

function isValidIPv4(raw: string): boolean {
  const parts = raw.split('.');
  if (parts.length !== 4) return false;
  return parts.every((p) => /^\d{1,3}$/.test(p) && Number(p) <= 255);
}

export function maskIP(ip: string, style: MaskStyle = 'dots'): string {
  const raw = ip.trim();
  if (!raw) return '—';
  const norm = canonicalIP(raw);
  const sep = style === 'star' ? '*' : '···';
  if (isValidIPv4(norm)) {
    return `${norm.split('.').slice(0, 2).join('.')}.${sep}.${sep}`;
  }
  const words = parseIPv6Words(raw);
  if (words) {
    if (words[0] === 0 && words[1] === 0) return `*:${sep}`;
    return `${words[0].toString(16)}:${words[1].toString(16)}:${sep}`;
  }
  return style === 'star' ? '***' : '···';
}

const IPV4_CANDIDATE_RE = /\b\d{1,3}(?:\.\d{1,3}){3}\b/g;
const IPV6_CANDIDATE_RE =
  /(?<![0-9a-zA-Z:])(?:[0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}(?:\.\d{1,3}\.\d{1,3}\.\d{1,3})?(?![0-9a-zA-Z:])/g;

export function maskIPsInText(text: string, style: MaskStyle = 'dots'): string {
  return text
    .replace(IPV6_CANDIDATE_RE, (token) => {
      if (token === '::') return token;
      if (parseIPv6Words(token)) return maskIP(token, style);
      if (token.endsWith(':') && !token.endsWith('::')) {
        const trimmed = token.slice(0, -1);
        if (trimmed !== '::' && parseIPv6Words(trimmed)) {
          return `${maskIP(trimmed, style)}:`;
        }
      }
      return token;
    })
    .replace(IPV4_CANDIDATE_RE, (token) => (isValidIPv4(token) ? maskIP(token, style) : token));
}
