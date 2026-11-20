export function maskIP(ip: string): string {
  const v4 = /^(?:\d{1,3}\.){3}\d{1,3}$/.exec(ip);
  if (v4) return `${ip.split('.').slice(0, 2).join('.')}.···.···`;
  const v6 = /^([0-9a-f]{1,4}:[0-9a-f]{1,4}):/i.exec(ip);
  return v6 ? `${v6[1]}:···` : ip ? '···' : '—';
}
