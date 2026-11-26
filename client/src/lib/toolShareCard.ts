import { getMessages, joinList, type Locale } from '../i18n';
import { canonicalIP, sameIP, sameIPv6Prefix64 } from './ip';
import { maskIP } from './ipDisplay';
import type { DNSExit } from './types';
import type { WebRTCVerdictReason, WebRTCVerdictState } from './webrtcCheck';
import type { IPv6VerdictReason, IPv6VerdictState, StackExit } from './ipv6Check';

const W = 1200;
const H = 840;
const SCALE = 2;
const PAD = 48;

const SANS = `Inter, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif`;
const MONO = `"JetBrains Mono", ui-monospace, Menlo, monospace`;

const C = {
  bg: '#020617',
  stage: '#030917',
  panel: 'rgba(15, 23, 42, 0.78)',
  line: 'rgba(255, 255, 255, 0.08)',
  grid: 'rgba(34, 197, 94, 0.03)',
  radar: '#4ade80',
  white: '#ffffff',
  slate100: '#f1f5f9',
  slate200: '#e2e8f0',
  slate300: '#cbd5e1',
  slate400: '#94a3b8',
  slate500: '#64748b',
  slate600: '#475569',
  sig: '#4ade80',
  warn: '#fbbf24',
  leak: '#f87171',
  unknown: '#94a3b8',
  sigSoft: 'rgba(74, 222, 128, 0.12)',
  warnSoft: 'rgba(251, 191, 36, 0.12)',
  leakSoft: 'rgba(248, 113, 113, 0.14)',
  unknownSoft: 'rgba(148, 163, 184, 0.12)',
};

const IPV4_RE = /\b(\d{1,3}\.\d{1,3})\.\d{1,3}\.\d{1,3}\b/g;
const IPV6_RE = /\b([0-9a-fA-F]{1,4}:[0-9a-fA-F]{1,4}):[0-9a-fA-F]{1,4}[0-9a-fA-F:]*\b/g;

function maskIPsInText(s: string): string {
  return s.replace(IPV4_RE, '$1.···.···').replace(IPV6_RE, '$1:···');
}

function font(size: number, weight = 400, family = SANS): string {
  return `${weight} ${size}px ${family}`;
}

function truncate(ctx: CanvasRenderingContext2D, value: string, maxW: number): string {
  if (maxW <= 12) return '…';
  if (ctx.measureText(value).width <= maxW) return value;
  let cut = value;
  while (cut.length > 1 && ctx.measureText(`${cut}…`).width > maxW) {
    cut = cut.slice(0, -1);
  }
  return `${cut}…`;
}

function drawText(
  ctx: CanvasRenderingContext2D,
  s: string,
  x: number,
  y: number,
  opts: {
    font?: string;
    color?: string;
    align?: CanvasTextAlign;
    baseline?: CanvasTextBaseline;
  } = {},
) {
  ctx.font = opts.font ?? font(14);
  ctx.fillStyle = opts.color ?? C.slate300;
  ctx.textAlign = opts.align ?? 'left';
  ctx.textBaseline = opts.baseline ?? 'middle';
  ctx.fillText(s, x, y);
}

function panel(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  fill = C.panel,
  radius = 14,
) {
  ctx.fillStyle = fill;
  ctx.strokeStyle = C.line;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, radius);
  ctx.fill();
  ctx.stroke();
}

function dot(ctx: CanvasRenderingContext2D, x: number, y: number, color: string, soft: string, r = 4.5) {
  if (soft !== 'transparent') {
    ctx.fillStyle = soft;
    ctx.beginPath();
    ctx.arc(x, y, r + 4, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fill();
}

function chip(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  label: string,
  color: string,
  soft: string,
  borderColor?: string,
  size = 11.5,
): number {
  ctx.font = font(size, 500, MONO);
  const w = ctx.measureText(label).width + 16;
  const h = size + 11;
  ctx.fillStyle = soft;
  ctx.beginPath();
  ctx.roundRect(x, y - h / 2, w, h, 6);
  ctx.fill();
  if (borderColor) {
    ctx.strokeStyle = borderColor;
    ctx.lineWidth = 1;
    ctx.stroke();
  }
  ctx.fillStyle = color;
  ctx.textAlign = 'left';
  ctx.textBaseline = 'middle';
  ctx.fillText(label, x + 8, y + 0.5);
  return w;
}

function drawBackground(ctx: CanvasRenderingContext2D) {
  ctx.fillStyle = C.bg;
  ctx.fillRect(0, 0, W, H);

  ctx.strokeStyle = C.grid;
  ctx.lineWidth = 1;
  for (let x = 0; x <= W; x += 40) {
    ctx.beginPath();
    ctx.moveTo(x + 0.5, 0);
    ctx.lineTo(x + 0.5, H);
    ctx.stroke();
  }
  for (let y = 0; y <= H; y += 40) {
    ctx.beginPath();
    ctx.moveTo(0, y + 0.5);
    ctx.lineTo(W, y + 0.5);
    ctx.stroke();
  }

  const g1 = ctx.createRadialGradient(0, 0, 0, 0, 0, 560);
  g1.addColorStop(0, 'rgba(34, 197, 94, 0.10)');
  g1.addColorStop(1, 'rgba(34, 197, 94, 0)');
  ctx.fillStyle = g1;
  ctx.fillRect(0, 0, W, H);

  const g2 = ctx.createRadialGradient(W, H, 0, W, H, 620);
  g2.addColorStop(0, 'rgba(37, 99, 235, 0.10)');
  g2.addColorStop(1, 'rgba(37, 99, 235, 0)');
  ctx.fillStyle = g2;
  ctx.fillRect(0, 0, W, H);
}

function formatStamp(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function drawHeader(ctx: CanvasRenderingContext2D, subtitle: string, timeMs: number) {
  const y = PAD + 6;
  drawText(ctx, 'Detect', PAD, y, { font: font(26, 700, MONO), color: C.white });
  const dw = ctx.measureText('Detect').width;
  drawText(ctx, 'Radar', PAD + dw, y, { font: font(26, 700, MONO), color: C.radar });
  drawText(ctx, subtitle, W - PAD, y - 9, { font: font(14, 500), color: C.slate400, align: 'right' });
  drawText(ctx, formatStamp(timeMs), W - PAD, y + 12, { font: font(12, 400, MONO), color: C.slate600, align: 'right' });
}

function drawFooter(ctx: CanvasRenderingContext2D, urlText: string, locale: Locale) {
  const fy = H - PAD + 14;
  dot(ctx, PAD + 4, fy, C.radar, C.sigSoft, 3.5);
  drawText(ctx, urlText, PAD + 18, fy, { font: font(14.5, 600, MONO), color: C.slate200 });
  drawText(ctx, getMessages(locale).common.shareFooterNote, W - PAD, fy, {
    font: font(12.5),
    color: C.slate500,
    align: 'right',
  });
}

function drawExitColumn(
  ctx: CanvasRenderingContext2D,
  lx: number,
  midY: number,
  maxW: number,
  exit: DNSExit | null,
  exitNetworkName: string,
  exitRegion: string,
  exitTypeLabel: string,
  locale: Locale,
) {
  const c = getMessages(locale).common;
  drawText(ctx, c.exitIP, lx, midY + 28, { font: font(13, 500), color: C.slate400 });
  if (exit?.ip) {
    ctx.font = font(16, 600);
    drawText(ctx, truncate(ctx, `${exitNetworkName}  ${exitRegion}`, maxW), lx, midY + 62, {
      font: font(16, 600),
      color: C.slate200,
    });
    ctx.font = font(28, 700, MONO);
    drawText(ctx, truncate(ctx, maskIP(exit.ip), maxW), lx, midY + 104, {
      font: font(28, 700, MONO),
      color: C.white,
    });
    let mx = lx;
    if (exitTypeLabel) {
      mx += chip(ctx, mx, midY + 146, exitTypeLabel, C.slate300, 'rgba(148,163,184,0.08)', C.line) + 10;
    }
    if (exit.asn) {
      drawText(ctx, exit.asn, mx, midY + 146, { font: font(12.5, 400, MONO), color: C.slate400 });
    }
  } else {
    drawText(ctx, c.unavailable, lx, midY + 72, { font: font(15), color: C.slate400 });
  }
}

function drawPolygon(
  ctx: CanvasRenderingContext2D,
  pts: readonly (readonly [number, number])[],
  fill: string,
  stroke?: string,
  lineWidth = 0.9,
) {
  ctx.beginPath();
  pts.forEach(([x, y], i) => (i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y)));
  ctx.closePath();
  ctx.fillStyle = fill;
  ctx.fill();
  if (stroke) {
    ctx.strokeStyle = stroke;
    ctx.lineWidth = lineWidth;
    ctx.stroke();
  }
}

function drawWireBadge(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  label: string,
  textColor: string,
  borderColor: string,
) {
  ctx.font = font(10.5, 500, MONO);
  const w = Math.max(40, ctx.measureText(label).width + 18);
  const h = 20;
  ctx.fillStyle = '#050d1d';
  ctx.strokeStyle = borderColor;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.roundRect(cx - w / 2, cy - h / 2, w, h, h / 2);
  ctx.fill();
  ctx.stroke();
  drawText(ctx, label, cx, cy + 0.5, { font: font(10.5, 500, MONO), color: textColor, align: 'center' });
}

interface FlowDiamondSpec {
  browserLabel: string;
  upperColor: string;
  upperTopFill: string;
  upperGlow: string;
  upperHalted: boolean;
  upperTitle: string;
  upperTitleColor: string;
  upperSub: string;
  upperSubColor: string;
  upperLeftBadge: string;
  upperRightBadge?: string;
  lowerExitLabel: string;
  lowerExitIP: string;
  lowerExitSubSuffix: string;
  lowerHalted?: boolean;
  lowerLeftBadge: string;
  lowerRightBadge?: string;
  rightColor: string;
  rightTitle: string;
  rightSub: string;
}

function drawFlowDiamondStage(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  spec: FlowDiamondSpec,
) {
  panel(ctx, x, y, w, h, C.stage, 14);
  ctx.save();
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, 14);
  ctx.clip();

  ctx.strokeStyle = 'rgba(148, 163, 184, 0.06)';
  ctx.lineWidth = 0.8;
  const dw = 40;
  const dh = 23.09;
  for (let gx = x - dw; gx < x + w + dw; gx += dw) {
    for (let gy = y - dh; gy < y + h + dh; gy += dh) {
      ctx.beginPath();
      ctx.moveTo(gx + dw / 2, gy);
      ctx.lineTo(gx + dw, gy + dh / 2);
      ctx.lineTo(gx + dw / 2, gy + dh);
      ctx.lineTo(gx, gy + dh / 2);
      ctx.closePath();
      ctx.stroke();
    }
  }

  const bx = x + 150;
  const cy = y + h / 2;
  const mx = x + w / 2;
  const ax = x + w - 150;
  const ry = y + 78;
  const ey = y + h - 66;

  ctx.save();
  ctx.lineWidth = 2.4;
  ctx.strokeStyle = spec.upperColor;
  if (spec.upperHalted) {
    ctx.setLineDash([5, 4]);
    ctx.beginPath();
    ctx.moveTo(bx, cy);
    ctx.lineTo(bx + 100, ry);
    ctx.lineTo(mx - 74, ry);
    ctx.stroke();
    ctx.setLineDash([]);
    drawText(ctx, '×', mx - 64, ry, { font: font(15, 700), color: C.unknown, align: 'center' });
  } else {
    ctx.beginPath();
    ctx.moveTo(bx, cy);
    ctx.lineTo(bx + 100, ry);
    ctx.lineTo(ax - 100, ry);
    ctx.lineTo(ax, cy);
    ctx.stroke();
  }

  ctx.strokeStyle = spec.lowerHalted ? C.unknown : C.sig;
  if (spec.lowerHalted) {
    ctx.setLineDash([5, 4]);
    ctx.beginPath();
    ctx.moveTo(bx, cy);
    ctx.lineTo(bx + 100, ey);
    ctx.lineTo(mx - 74, ey);
    ctx.stroke();
    ctx.setLineDash([]);
    drawText(ctx, '×', mx - 64, ey, { font: font(15, 700), color: C.unknown, align: 'center' });
  } else {
    ctx.beginPath();
    ctx.moveTo(bx, cy);
    ctx.lineTo(bx + 100, ey);
    ctx.lineTo(ax - 100, ey);
    ctx.lineTo(ax, cy);
    ctx.stroke();
  }
  ctx.restore();

  drawWireBadge(ctx, bx + 204, ry + 22, spec.upperLeftBadge, C.slate300, spec.upperColor);
  if (spec.upperRightBadge) {
    drawWireBadge(ctx, ax - 204, ry + 22, spec.upperRightBadge, C.slate300, 'rgba(148,163,184,0.3)');
  }
  const lowerWireColor = spec.lowerHalted ? C.unknown : '#86efac';
  const lowerBadgeBorder = spec.lowerHalted ? 'rgba(148,163,184,0.35)' : 'rgba(74,222,128,0.42)';
  drawWireBadge(ctx, bx + 204, ey - 22, spec.lowerLeftBadge, lowerWireColor, lowerBadgeBorder);
  if (spec.lowerRightBadge) {
    drawWireBadge(ctx, ax - 204, ey - 22, spec.lowerRightBadge, lowerWireColor, 'rgba(148,163,184,0.3)');
  }

  ctx.fillStyle = 'rgba(0,0,0,0.48)';
  ctx.beginPath();
  ctx.ellipse(bx, cy + 18, 30, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  drawPolygon(ctx, [[bx - 24, cy + 6], [bx, cy - 6], [bx + 26, cy + 6], [bx + 2, cy + 18]], '#293548', '#475569');
  drawPolygon(ctx, [[bx - 24, cy + 6], [bx + 2, cy + 18], [bx + 2, cy + 22], [bx - 24, cy + 10]], '#141e30', '#334155');
  drawPolygon(ctx, [[bx - 24, cy + 6], [bx - 24, cy - 20], [bx, cy - 32], [bx, cy - 6]], '#0f172a', '#64748b');
  drawPolygon(ctx, [[bx - 21, cy + 3], [bx - 21, cy - 17], [bx - 3, cy - 26], [bx - 3, cy - 6]], '#0f2942');
  drawText(ctx, spec.browserLabel, bx, cy + 38, { font: font(12, 600), color: C.slate200, align: 'center' });

  ctx.fillStyle = spec.upperGlow;
  ctx.beginPath();
  ctx.ellipse(mx, ry + 20, 30, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  drawPolygon(ctx, [[mx, ry - 22], [mx + 22, ry - 11], [mx, ry], [mx - 22, ry - 11]], spec.upperTopFill, spec.upperColor);
  drawPolygon(ctx, [[mx - 22, ry - 11], [mx, ry], [mx, ry + 18], [mx - 22, ry + 7]], '#182234', '#334155');
  drawPolygon(ctx, [[mx, ry], [mx + 22, ry - 11], [mx + 22, ry + 7], [mx, ry + 18]], '#0b1220', '#1e293b');
  ctx.strokeStyle = spec.upperColor;
  ctx.lineWidth = 1.6;
  ctx.beginPath();
  ctx.moveTo(mx, ry - 11);
  ctx.lineTo(mx, ry - 26);
  ctx.stroke();
  ctx.fillStyle = spec.upperColor;
  ctx.beginPath();
  ctx.arc(mx, ry - 27, 2.8, 0, Math.PI * 2);
  ctx.fill();

  ctx.font = font(12.5, 600);
  drawText(ctx, truncate(ctx, spec.upperTitle, 260), mx, ry - 56, {
    font: font(12.5, 600),
    color: spec.upperTitleColor,
    align: 'center',
  });
  const upperSubIsIP = /^(?:\d{1,3}\.){2}|^[0-9a-fA-F]{1,4}:/.test(spec.upperSub);
  const upperSubFont = font(11, 400, upperSubIsIP ? MONO : SANS);
  ctx.font = upperSubFont;
  drawText(ctx, truncate(ctx, spec.upperSub, 260), mx, ry - 41, {
    font: upperSubFont,
    color: spec.upperSubColor,
    align: 'center',
  });

  ctx.fillStyle = 'rgba(74, 222, 128, 0.14)';
  ctx.beginPath();
  ctx.ellipse(mx, ey + 20, 28, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  drawPolygon(ctx, [[mx, ey - 26], [mx + 21, ey - 16], [mx, ey - 6], [mx - 21, ey - 16]], '#12402b', C.sig);
  drawPolygon(ctx, [[mx - 21, ey - 16], [mx, ey - 6], [mx, ey + 18], [mx - 21, ey + 8]], '#182234', '#334155');
  drawPolygon(ctx, [[mx, ey - 6], [mx + 21, ey - 16], [mx + 21, ey + 8], [mx, ey + 18]], '#0b1220', '#1e293b');
  ctx.font = font(12.5, 600);
  drawText(ctx, truncate(ctx, spec.lowerExitLabel, 260), mx, ey + 34, {
    font: font(12.5, 600),
    color: C.white,
    align: 'center',
  });
  drawText(ctx, `${spec.lowerExitIP} · ${spec.lowerExitSubSuffix}`, mx, ey + 50, {
    font: font(11, 400, MONO),
    color: C.slate400,
    align: 'center',
  });

  ctx.strokeStyle = spec.rightColor;
  ctx.lineWidth = 1.4;
  ctx.globalAlpha = 0.6;
  ctx.beginPath();
  ctx.ellipse(ax, cy + 14, 38, 15, 0, 0, Math.PI * 2);
  ctx.stroke();
  ctx.globalAlpha = 0.85;
  ctx.beginPath();
  ctx.ellipse(ax, cy + 14, 28, 11, 0, 0, Math.PI * 2);
  ctx.stroke();
  ctx.globalAlpha = 1;
  drawPolygon(ctx, [[ax, cy - 10], [ax + 24, cy + 2], [ax, cy + 14], [ax - 24, cy + 2]], '#293548', spec.rightColor);
  drawPolygon(ctx, [[ax - 24, cy + 2], [ax, cy + 14], [ax, cy + 20], [ax - 24, cy + 8]], '#182234', '#334155');
  drawPolygon(ctx, [[ax, cy + 14], [ax + 24, cy + 2], [ax + 24, cy + 8], [ax, cy + 20]], '#0b1220', '#1e293b');
  ctx.fillStyle = '#040d1d';
  ctx.strokeStyle = spec.rightColor;
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.arc(ax, cy - 8, 11, 0, Math.PI * 2);
  ctx.fill();
  ctx.stroke();
  ctx.fillStyle = spec.rightColor;
  ctx.beginPath();
  ctx.arc(ax, cy - 8, 4.5, 0, Math.PI * 2);
  ctx.fill();
  drawText(ctx, spec.rightTitle, ax, cy + 38, { font: font(12, 600), color: C.white, align: 'center' });
  drawText(ctx, spec.rightSub, ax, cy + 54, { font: font(11, 600), color: spec.rightColor, align: 'center' });

  ctx.restore();
}

function drawVerdictBand(
  ctx: CanvasRenderingContext2D,
  cardY: number,
  cardW: number,
  color: string,
  soft: string,
  title: string,
  note: string,
  locale: Locale,
) {
  const vg = ctx.createLinearGradient(PAD, cardY, PAD + cardW * 0.66, cardY + 102);
  vg.addColorStop(0, soft);
  vg.addColorStop(1, 'rgba(0,0,0,0)');
  ctx.save();
  ctx.beginPath();
  ctx.roundRect(PAD, cardY, cardW, 102, [14, 14, 0, 0]);
  ctx.clip();
  ctx.fillStyle = vg;
  ctx.fillRect(PAD, cardY, cardW, 102);
  ctx.restore();

  drawText(ctx, getMessages(locale).common.thisScan, PAD + 28, cardY + 24, { font: font(12.5, 500), color: C.slate400 });
  dot(ctx, PAD + 34, cardY + 52, color, soft, 5);
  drawText(ctx, title, PAD + 50, cardY + 52, { font: font(22, 700), color: C.white });
  ctx.font = font(14);
  drawText(ctx, truncate(ctx, maskIPsInText(note), cardW - 56), PAD + 28, cardY + 82, {
    font: font(14),
    color: C.slate300,
  });
}

async function toPngBlob(canvas: HTMLCanvasElement, locale: Locale): Promise<Blob> {
  const errMsg = getMessages(locale).common.imageEmptyError;
  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error(errMsg))), 'image/png');
  });
}

export async function saveShareImage(
  blob: Blob,
  prefix: string,
  locale: Locale = 'zh-CN',
): Promise<string> {
  const c = getMessages(locale).common;
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${prefix}-${new Date().toISOString().replace(/[:.]/g, '-')}.png`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);

  let clipboardOk = false;
  try {
    clipboardOk = await Promise.race([
      navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]).then(() => true),
      new Promise<boolean>((resolve) => setTimeout(() => resolve(false), 450)),
    ]);
  } catch {
  }
  return clipboardOk ? c.imageCopiedAndSaved : c.imageSavedOnly;
}

export interface DNSShareProviderRow {
  name: string;
  regions: string;
  type?: 'public' | 'access' | 'hosting';
  suspect: boolean;
}

export interface DNSShareCardInput {
  time: number;
  verdictState: 'suspected' | 'clear' | 'unknown' | 'unobserved';
  verdictTitle: string;
  note: string;
  exit: DNSExit | null;
  exitNetworkName: string;
  exitRegion: string;
  exitTypeLabel: string;
  providers: DNSShareProviderRow[];
  resolverCount: number;
  locale?: Locale;
}

const DNS_VERDICT_PALETTE: Record<DNSShareCardInput['verdictState'], { color: string; soft: string; glow: string }> = {
  suspected: { color: C.leak, soft: C.leakSoft, glow: 'rgba(248, 113, 113, 0.16)' },
  clear: { color: C.sig, soft: C.sigSoft, glow: 'rgba(74, 222, 128, 0.14)' },
  unknown: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
  unobserved: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
};

export async function renderDNSShareCard(data: DNSShareCardInput): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;
  const locale: Locale = data.locale ?? 'zh-CN';
  const m = getMessages(locale);
  const fl = m.dns.flow;
  const pg = m.dns.page;
  const sh = m.dns.share;

  const canvas = document.createElement('canvas');
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);
  drawHeader(ctx, sh.cardSubtitle, data.time);

  const pal = DNS_VERDICT_PALETTE[data.verdictState];
  const unobserved = data.verdictState === 'unobserved';
  const resNames = data.verdictState === 'suspected'
    ? data.providers.filter((p) => p.suspect).map((p) => p.name)
    : data.providers.map((p) => p.name);
  const resUnique = [...new Set((resNames.length ? resNames : data.providers.map((p) => p.name)).filter(Boolean))];
  const topNames = joinList(resUnique.slice(0, 2), locale);
  const upperTitle = unobserved
    ? fl.resolverUnobserved
    : resUnique.length
      ? resUnique.length > 2
        ? fl.moreProviders(topNames)
        : topNames
      : fl.resolverUnknown;
  const upperSub = data.verdictState === 'suspected'
    ? fl.subSuspected
    : data.verdictState === 'clear'
      ? fl.subClearCount(data.resolverCount)
      : unobserved
        ? fl.subUnobserved
        : pg.addressCount(data.resolverCount);

  drawFlowDiamondStage(ctx, PAD, 102, W - PAD * 2, 250, {
    browserLabel: fl.browserNode,
    upperColor: pal.color,
    upperTopFill: data.verdictState === 'suspected' ? '#5c1922' : data.verdictState === 'clear' ? '#12402b' : '#293548',
    upperGlow: pal.glow,
    upperHalted: unobserved,
    upperTitle,
    upperTitleColor: data.verdictState === 'suspected' ? '#fca5a5' : C.white,
    upperSub,
    upperSubColor: data.verdictState === 'suspected' ? C.leak : C.slate400,
    upperLeftBadge: fl.wireDns,
    upperRightBadge: unobserved ? fl.wireNoCapture : fl.wireAuth,
    lowerExitLabel: data.exit?.ip ? `${data.exitNetworkName} · ${data.exitRegion}` : m.common.unavailable,
    lowerExitIP: data.exit?.ip ? maskIP(data.exit.ip) : '—',
    lowerExitSubSuffix: fl.exitSuffix,
    lowerLeftBadge: fl.wireHttps,
    lowerRightBadge: fl.wireApi,
    rightColor: pal.color,
    rightTitle: fl.authorityNode,
    rightSub: data.verdictTitle,
  });

  const cardY = 368;
  const cardW = W - PAD * 2;
  const cardH = H - cardY - PAD - 28;
  panel(ctx, PAD, cardY, cardW, cardH);
  drawVerdictBand(ctx, cardY, cardW, pal.color, pal.soft, data.verdictTitle, data.note, locale);

  const midY = cardY + 102;
  const splitX = PAD + 420;
  ctx.strokeStyle = C.line;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(PAD, midY);
  ctx.lineTo(PAD + cardW, midY);
  ctx.moveTo(splitX, midY);
  ctx.lineTo(splitX, cardY + cardH);
  ctx.stroke();

  drawExitColumn(ctx, PAD + 28, midY, splitX - PAD - 56, data.exit, data.exitNetworkName, data.exitRegion, data.exitTypeLabel, locale);

  const rx = splitX + 28;
  const rightW = PAD + cardW - 28 - rx;
  drawText(ctx, 'DNS', rx, midY + 28, { font: font(13, 500), color: C.slate400 });
  drawText(ctx, pg.addressCount(data.resolverCount), rx + rightW, midY + 28, {
    font: font(12.5, 400, MONO),
    color: C.slate400,
    align: 'right',
  });

  if (data.providers.length === 0) {
    drawText(ctx, sh.noRecords, rx, midY + 72, { font: font(14), color: C.slate400 });
  } else {
    const shown = data.providers.slice(0, 4);
    shown.forEach((p, idx) => {
      const rowY = midY + 72 + idx * 46;
      if (idx > 0) {
        ctx.strokeStyle = 'rgba(255,255,255,0.05)';
        ctx.beginPath();
        ctx.moveTo(rx, rowY - 23);
        ctx.lineTo(rx + rightW, rowY - 23);
        ctx.stroke();
      }
      let nx = rx;
      if (p.suspect) {
        dot(ctx, nx + 4, rowY, C.leak, 'transparent', 3);
        nx += 14;
      }
      const tag = p.type === 'public' ? pg.tagPublic : p.type === 'access' ? pg.tagAccess : '';
      ctx.font = font(11.5, 500, MONO);
      const tagW = tag ? ctx.measureText(tag).width + 24 : 0;
      ctx.font = font(15, 600);
      const nameStr = truncate(ctx, p.name, rightW - (nx - rx) - tagW - 96);
      drawText(ctx, nameStr, nx, rowY, { font: font(15, 600), color: p.suspect ? C.leak : C.slate200 });
      const nw = ctx.measureText(nameStr).width;
      if (p.regions) {
        ctx.font = font(13);
        const regMaxW = Math.max(40, rightW - (nx - rx) - nw - tagW - 18);
        drawText(ctx, truncate(ctx, p.regions, regMaxW), nx + nw + 10, rowY, { font: font(13), color: C.slate400 });
      }
      if (tag) {
        ctx.font = font(11.5, 500, MONO);
        const cw = ctx.measureText(tag).width + 16;
        chip(ctx, rx + rightW - cw, rowY, tag, C.slate300, 'rgba(148,163,184,0.08)', C.line);
      }
    });
    if (data.providers.length > shown.length) {
      drawText(ctx, sh.moreProviders(data.providers.length - shown.length), rx, cardY + cardH - 18, {
        font: font(12),
        color: C.slate500,
      });
    }
  }

  drawFooter(ctx, locale === 'en' ? 'detectradar.com/en/dns-leak-test/' : 'detectradar.com/dns-leak-test/', locale);
  return toPngBlob(canvas, locale);
}

export interface WebRTCShareCardInput {
  time: number;
  verdictState: WebRTCVerdictState;
  verdictTitle: string;
  verdictReason: WebRTCVerdictReason;
  verdictNote: string;
  exit: DNSExit | null;
  exitNetworkName: string;
  exitRegion: string;
  exitTypeLabel: string;
  apiSupported: boolean;
  publicIPs: string[];
  suspectIPs: string[];
  localIPs: { ip: string; scope: string }[];
  mdnsCount: number;
  ipv6Global: string | null;
  locale?: Locale;
}

const WEBRTC_VERDICT_PALETTE: Record<WebRTCVerdictState, { color: string; soft: string; glow: string }> = {
  leaked: { color: C.leak, soft: C.leakSoft, glow: 'rgba(248, 113, 113, 0.16)' },
  clear: { color: C.sig, soft: C.sigSoft, glow: 'rgba(74, 222, 128, 0.14)' },
  unknown: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
};

function webrtcUpperCopy(data: WebRTCShareCardInput, locale: Locale): { title: string; sub: string } {
  const fl = getMessages(locale).webrtc.flow;
  switch (data.verdictReason) {
    case 'disabled':
      return { title: fl.titleDisabled, sub: fl.subDisabled };
    case 'no-candidates':
      return { title: fl.titleNoCands, sub: fl.subNoCands };
    case 'mdns-protected':
      return { title: fl.titleMdns, sub: fl.subMdns(data.mdnsCount) };
    case 'local-exposed':
      return {
        title: fl.titleLocal,
        sub: data.localIPs[0] ? fl.subLocalSingle(maskIP(data.localIPs[0].ip)) : fl.subLocalFallback,
      };
    case 'public-leak': {
      const ip = data.suspectIPs[0] ?? data.publicIPs[0];
      return {
        title: fl.titlePublic,
        sub: ip ? fl.subLeakSingle(maskIP(ip)) : fl.subLeakFallback,
      };
    }
    case 'dual-stack':
      return {
        title: fl.titlePublic,
        sub: fl.subDual(data.publicIPs[0] ? maskIP(data.publicIPs[0]) : undefined),
      };
    case 'same-prefix':
      return {
        title: fl.titleIPv6,
        sub: fl.subPrefix(data.publicIPs[0] ? maskIP(data.publicIPs[0]) : undefined),
      };
    case 'same-network':
      return {
        title: fl.titleSameNet,
        sub: fl.subSameNet(data.publicIPs[0] ? maskIP(data.publicIPs[0]) : undefined),
      };
    case 'assessment-unavailable':
      return {
        title: fl.titlePublic,
        sub: fl.subUnavail(data.publicIPs[0] ? maskIP(data.publicIPs[0]) : undefined),
      };
    case 'missing-exit':
      return {
        title: fl.titlePublic,
        sub: data.publicIPs[0] ? maskIP(data.publicIPs[0]) : fl.subMissingExit,
      };
    case 'matches-exit':
    default:
      return {
        title: fl.titlePublic,
        sub: fl.subMatch(data.publicIPs[0] ? maskIP(data.publicIPs[0]) : undefined),
      };
  }
}

export async function renderWebRTCShareCard(data: WebRTCShareCardInput): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;
  const locale: Locale = data.locale ?? 'zh-CN';
  const m = getMessages(locale);
  const fl = m.webrtc.flow;
  const pg = m.webrtc.page;
  const sh = m.webrtc.share;

  const canvas = document.createElement('canvas');
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);
  drawHeader(ctx, sh.cardSubtitle, data.time);

  const pal = WEBRTC_VERDICT_PALETTE[data.verdictState];
  const halted = !data.apiSupported || data.verdictReason === 'no-candidates';
  const upperColor = halted ? C.unknown : pal.color;
  const upperTopFill =
    data.verdictState === 'leaked'
      ? '#5c1922'
      : !halted && data.verdictState === 'clear'
        ? '#12402b'
        : '#293548';
  const upperCopy = webrtcUpperCopy(data, locale);

  drawFlowDiamondStage(ctx, PAD, 102, W - PAD * 2, 250, {
    browserLabel: fl.browserNode,
    upperColor,
    upperTopFill,
    upperGlow: halted ? 'rgba(148, 163, 184, 0.12)' : pal.glow,
    upperHalted: halted,
    upperTitle: upperCopy.title,
    upperTitleColor: data.verdictState === 'leaked' ? '#fca5a5' : C.white,
    upperSub: upperCopy.sub,
    upperSubColor: data.verdictState === 'leaked' ? C.leak : C.slate400,
    upperLeftBadge: fl.wireStun,
    upperRightBadge: halted ? fl.wireNoReturn : fl.wireLocalRead,
    lowerExitLabel: data.exit?.ip ? `${data.exitNetworkName} · ${data.exitRegion}` : m.common.unavailable,
    lowerExitIP: data.exit?.ip ? maskIP(data.exit.ip) : '—',
    lowerExitSubSuffix: m.dns.flow.exitSuffix,
    lowerLeftBadge: fl.wireHttps,
    lowerRightBadge: fl.wireApi,
    rightColor: pal.color,
    rightTitle: fl.judgeNode,
    rightSub: data.verdictTitle,
  });

  const cardY = 368;
  const cardW = W - PAD * 2;
  const cardH = H - cardY - PAD - 28;
  panel(ctx, PAD, cardY, cardW, cardH);
  drawVerdictBand(ctx, cardY, cardW, pal.color, pal.soft, data.verdictTitle, data.verdictNote, locale);

  const midY = cardY + 102;
  const splitX = PAD + 420;
  ctx.strokeStyle = C.line;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(PAD, midY);
  ctx.lineTo(PAD + cardW, midY);
  ctx.moveTo(splitX, midY);
  ctx.lineTo(splitX, cardY + cardH);
  ctx.stroke();

  const lx = PAD + 28;
  drawExitColumn(ctx, lx, midY, splitX - PAD - 56, data.exit, data.exitNetworkName, data.exitRegion, data.exitTypeLabel, locale);

  ctx.strokeStyle = C.line;
  ctx.beginPath();
  ctx.moveTo(lx, midY + 180);
  ctx.lineTo(splitX - 28, midY + 180);
  ctx.stroke();

  const stats: [string, string][] = [
    [sh.statPublic, sh.countUnit(data.publicIPs.length)],
    [sh.statLocal, sh.countUnit(data.localIPs.length)],
    [sh.statMdns, sh.countUnit(data.mdnsCount)],
  ];
  stats.forEach(([label, val], idx) => {
    const sy = midY + 206 + idx * 28;
    drawText(ctx, label, lx, sy, { font: font(13), color: C.slate400 });
    drawText(ctx, val, splitX - 28, sy, { font: font(13, 600, MONO), color: C.slate200, align: 'right' });
  });

  const rx = splitX + 28;
  const rightW = PAD + cardW - 28 - rx;
  const suspectSet = new Set(data.suspectIPs.map((ip) => canonicalIP(ip)));
  drawText(ctx, 'WebRTC', rx, midY + 28, { font: font(13, 500), color: C.slate400 });

  const pubShown = data.apiSupported ? Math.min(2, data.publicIPs.length) : 0;
  const r1Y = midY + 66;
  drawText(ctx, pg.publicIPLabel, rx, r1Y, { font: font(14, 500), color: C.slate200 });
  if (!data.apiSupported) {
    ctx.font = font(11.5, 500, MONO);
    const cw = ctx.measureText(pg.disabledBadge).width + 16;
    chip(ctx, rx + rightW - cw, r1Y, pg.disabledBadge, C.slate300, 'rgba(148,163,184,0.08)', C.line);
  } else if (data.publicIPs.length === 0) {
    drawText(ctx, pg.noPublicIP, rx + rightW, r1Y, { font: font(13), color: C.slate400, align: 'right' });
  } else {
    data.publicIPs.slice(0, 2).forEach((ip, idx) => {
      const canon = canonicalIP(ip);
      const iy = r1Y + 28 + idx * 26;
      const isSuspect = suspectSet.has(canon);
      const matchesExit = sameIP(canon, data.exit?.ip);
      const samePrefix = !matchesExit && sameIPv6Prefix64(canon, data.exit?.ip);
      let tx = rx;
      if (isSuspect) {
        dot(ctx, tx + 4, iy, C.leak, 'transparent', 3);
        tx += 14;
      }
      drawText(ctx, maskIP(canon), tx, iy, {
        font: font(14, 500, MONO),
        color: isSuspect ? C.leak : C.slate200,
      });
      const tag = isSuspect
        ? pg.tagDifferent
        : matchesExit
          ? pg.tagMatch
          : samePrefix
            ? pg.tagSamePrefix
            : data.verdictReason === 'dual-stack'
              ? pg.tagSameISP
              : data.verdictReason === 'same-network'
                ? pg.tagSharedExit
                : '';
      if (tag) {
        ctx.font = font(11.5, 500, MONO);
        const tw = ctx.measureText(tag).width + 16;
        chip(
          ctx,
          rx + rightW - tw,
          iy,
          tag,
          isSuspect ? C.leak : C.slate300,
          isSuspect ? C.leakSoft : 'rgba(148,163,184,0.08)',
          isSuspect ? 'rgba(248,113,113,0.35)' : C.line,
        );
      }
    });
  }

  const sep1Y = pubShown > 0 ? r1Y + 28 + pubShown * 26 + 10 : r1Y + 34;
  ctx.strokeStyle = C.line;
  ctx.beginPath();
  ctx.moveTo(rx, sep1Y);
  ctx.lineTo(rx + rightW, sep1Y);
  ctx.stroke();

  const localShown = data.apiSupported ? Math.min(2, data.localIPs.length) : 0;
  const r2Y = sep1Y + 30;
  drawText(ctx, pg.localAddrLabel, rx, r2Y, { font: font(14, 500), color: C.slate200 });
  if (!data.apiSupported) {
    drawText(ctx, pg.apiDisabled, rx + rightW, r2Y, { font: font(13), color: C.slate400, align: 'right' });
  } else if (data.localIPs.length === 0 && data.mdnsCount > 0) {
    const label = pg.mdnsHidden;
    ctx.font = font(11.5, 500, MONO);
    const cw = ctx.measureText(label).width + 16;
    chip(ctx, rx + rightW - cw, r2Y, label, C.sig, C.sigSoft, 'rgba(74,222,128,0.35)');
  } else if (data.localIPs.length === 0) {
    drawText(ctx, pg.localNotExposed, rx + rightW, r2Y, { font: font(13), color: C.slate400, align: 'right' });
  } else {
    data.localIPs.slice(0, 2).forEach((item, idx) => {
      const iy = r2Y + 28 + idx * 24;
      drawText(ctx, maskIP(item.ip), rx, iy, { font: font(14, 500, MONO), color: C.slate200 });
      ctx.font = font(11.5, 500, MONO);
      const cw = ctx.measureText(item.scope).width + 16;
      chip(ctx, rx + rightW - cw, iy, item.scope, C.slate300, 'rgba(148,163,184,0.08)', C.line);
    });
  }

  const sep2Y = localShown > 0 ? r2Y + 28 + localShown * 24 + 10 : r2Y + 34;
  ctx.strokeStyle = C.line;
  ctx.beginPath();
  ctx.moveTo(rx, sep2Y);
  ctx.lineTo(rx + rightW, sep2Y);
  ctx.stroke();

  const r3Y = sep2Y + 30;
  drawText(ctx, pg.ipv6AddrLabel, rx, r3Y, { font: font(14, 500), color: C.slate200 });
  if (data.ipv6Global) {
    drawText(ctx, maskIP(data.ipv6Global), rx + rightW, r3Y, {
      font: font(14, 500, MONO),
      color: C.slate200,
      align: 'right',
    });
  } else {
    drawText(ctx, data.apiSupported ? pg.ipv6None : sh.ipv6SkippedCard, rx + rightW, r3Y, {
      font: font(13),
      color: C.slate400,
      align: 'right',
    });
  }

  drawFooter(ctx, locale === 'en' ? 'detectradar.com/en/webrtc-leak-test/' : 'detectradar.com/webrtc-leak-test/', locale);
  return toPngBlob(canvas, locale);
}

export interface IPv6ShareCardInput {
  time: number;
  verdictState: IPv6VerdictState;
  verdictTitle: string;
  verdictReason: IPv6VerdictReason;
  verdictNote: string;
  v4: StackExit | null;
  v4NetworkName: string;
  v4Region: string;
  v4TypeLabel: string;
  v6: StackExit | null;
  v6NetworkName: string;
  v6Region: string;
  v6TypeLabel: string;
  locale?: Locale;
}

const IPV6_VERDICT_PALETTE: Record<IPv6VerdictState, { color: string; soft: string; glow: string }> = {
  leaked: { color: C.leak, soft: C.leakSoft, glow: 'rgba(248, 113, 113, 0.16)' },
  clear: { color: C.sig, soft: C.sigSoft, glow: 'rgba(74, 222, 128, 0.14)' },
  unknown: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
};

function drawStackColumn(
  ctx: CanvasRenderingContext2D,
  x: number,
  midY: number,
  maxW: number,
  heading: string,
  exit: StackExit | null,
  networkName: string,
  region: string,
  typeLabel: string,
  locale: Locale,
  opts: { accent?: string; statusChip?: { label: string; color: string; soft: string; border: string }; emptyText: string },
) {
  const sh = getMessages(locale).ipv6.share;
  drawText(ctx, heading, x, midY + 28, { font: font(13, 500), color: C.slate400 });
  if (exit?.ip) {
    ctx.font = font(16, 600);
    drawText(ctx, truncate(ctx, `${networkName}  ${region}`, maxW), x, midY + 62, {
      font: font(16, 600),
      color: C.slate200,
    });
    ctx.font = font(26, 700, MONO);
    drawText(ctx, truncate(ctx, maskIP(exit.ip), maxW), x, midY + 104, {
      font: font(26, 700, MONO),
      color: opts.accent ?? C.white,
    });
    let mx = x;
    if (opts.statusChip) {
      mx += chip(ctx, mx, midY + 146, opts.statusChip.label, opts.statusChip.color, opts.statusChip.soft, opts.statusChip.border) + 8;
    }
    if (typeLabel) {
      mx += chip(ctx, mx, midY + 146, typeLabel, C.slate300, 'rgba(148,163,184,0.08)', C.line) + 10;
    }
    if (exit.asn) {
      drawText(ctx, exit.asn, mx, midY + 146, { font: font(12.5, 400, MONO), color: C.slate400 });
    }
    if (typeof exit.latencyMs === 'number') {
      drawText(ctx, sh.probeLatency(exit.latencyMs), x, midY + 186, {
        font: font(12.5, 400, MONO),
        color: C.slate500,
      });
    }
  } else {
    drawText(ctx, opts.emptyText, x, midY + 72, { font: font(15), color: C.slate400 });
  }
}

export async function renderIPv6ShareCard(data: IPv6ShareCardInput): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;
  const locale: Locale = data.locale ?? 'zh-CN';
  const m = getMessages(locale);
  const fl = m.ipv6.flow;
  const pg = m.ipv6.page;
  const sh = m.ipv6.share;

  const canvas = document.createElement('canvas');
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);
  drawHeader(ctx, sh.cardSubtitle, data.time);

  const pal = IPV6_VERDICT_PALETTE[data.verdictState];
  const v6Halted = !data.v6?.ip;
  const upperColor = data.verdictState === 'leaked' ? C.leak : v6Halted ? C.unknown : C.sig;
  const upperTopFill =
    data.verdictState === 'leaked'
      ? '#5c1922'
      : !v6Halted
        ? '#12402b'
        : '#293548';

  const upperTitle = data.v6?.ip
    ? `${data.v6NetworkName} · ${data.v6Region}`
    : fl.v6Disconnected;
  const upperSub = data.v6?.ip
    ? `${maskIP(data.v6.ip)}${data.verdictState === 'leaked' ? sh.cardLeakSuffix : ''}`
    : sh.cardV6OnlyV4;

  drawFlowDiamondStage(ctx, PAD, 102, W - PAD * 2, 250, {
    browserLabel: fl.browserNode,
    upperColor,
    upperTopFill,
    upperGlow: v6Halted ? 'rgba(148, 163, 184, 0.12)' : data.verdictState === 'leaked' ? C.leakSoft : C.sigSoft,
    upperHalted: v6Halted,
    upperTitle,
    upperTitleColor: data.verdictState === 'leaked' ? '#fca5a5' : C.white,
    upperSub,
    upperSubColor: data.verdictState === 'leaked' ? C.leak : C.slate400,
    upperLeftBadge: fl.wireV6,
    upperRightBadge: v6Halted ? fl.wireDisconnected : '6.detectradar',
    lowerExitLabel: data.v4?.ip ? `${data.v4NetworkName} · ${data.v4Region}` : fl.v4Disconnected,
    lowerExitIP: data.v4?.ip ? maskIP(data.v4.ip) : '—',
    lowerExitSubSuffix: pg.v4Label,
    lowerHalted: !data.v4?.ip,
    lowerLeftBadge: fl.wireV4,
    lowerRightBadge: data.v4?.ip ? '4.detectradar' : fl.wireDisconnected,
    rightColor: pal.color,
    rightTitle: sh.cardJudgeTitle,
    rightSub: data.verdictTitle,
  });

  const cardY = 368;
  const cardW = W - PAD * 2;
  const cardH = H - cardY - PAD - 28;
  panel(ctx, PAD, cardY, cardW, cardH);
  drawVerdictBand(ctx, cardY, cardW, pal.color, pal.soft, data.verdictTitle, data.verdictNote, locale);

  const midY = cardY + 102;
  const splitX = PAD + cardW / 2;
  ctx.strokeStyle = C.line;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(PAD, midY);
  ctx.lineTo(PAD + cardW, midY);
  ctx.moveTo(splitX, midY);
  ctx.lineTo(splitX, cardY + cardH);
  ctx.stroke();

  const colMaxW = cardW / 2 - 56;
  drawStackColumn(
    ctx,
    PAD + 28,
    midY,
    colMaxW,
    pg.v4Label,
    data.v4,
    data.v4NetworkName,
    data.v4Region,
    data.v4TypeLabel,
    locale,
    { emptyText: pg.v4Unreachable },
  );

  const v6Chip = data.v6?.ip
    ? data.verdictState === 'leaked'
      ? { label: pg.badgeLeak, color: C.leak, soft: C.leakSoft, border: 'rgba(248,113,113,0.35)' }
      : data.verdictReason === 'overseas-split'
        ? { label: pg.badgeSplit, color: C.sig, soft: C.sigSoft, border: 'rgba(74,222,128,0.35)' }
        : undefined
    : undefined;

  drawStackColumn(
    ctx,
    splitX + 28,
    midY,
    colMaxW,
    pg.v6Label,
    data.v6,
    data.v6NetworkName,
    data.v6Region,
    data.v6TypeLabel,
    locale,
    {
      accent: data.verdictState === 'leaked' ? C.leak : C.white,
      statusChip: v6Chip,
      emptyText: pg.v6Unreachable,
    },
  );

  drawFooter(ctx, locale === 'en' ? 'detectradar.com/en/ipv6-leak-test/' : 'detectradar.com/ipv6-leak-test/', locale);
  return toPngBlob(canvas, locale);
}
