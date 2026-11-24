/**
 * 专项检测页（DNS / WebRTC）本地 Canvas 2D 分享图渲染器。
 *
 * 所有 IP 均经 maskIP / maskIPsInText 自动脱敏后绘制，与首页 shareCard.ts 保持同源视觉语言与隐私边界。
 */

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

function drawFooter(ctx: CanvasRenderingContext2D, urlText: string) {
  const fy = H - PAD + 14;
  drawText(ctx, urlText, PAD, fy, { font: font(13, 500, MONO), color: C.slate500 });
  drawText(ctx, 'IP 已自动脱敏 · 本地生成', W - PAD, fy, { font: font(12), color: C.slate600, align: 'right' });
}

function drawExitColumn(
  ctx: CanvasRenderingContext2D,
  lx: number,
  midY: number,
  exit: DNSExit | null,
  exitNetworkName: string,
  exitRegion: string,
  exitTypeLabel: string,
) {
  drawText(ctx, '出口 IP', lx, midY + 28, { font: font(13, 500), color: C.slate400 });
  if (exit?.ip) {
    drawText(ctx, `${exitNetworkName}  ${exitRegion}`, lx, midY + 62, {
      font: font(16, 600),
      color: C.slate200,
    });
    drawText(ctx, maskIP(exit.ip), lx, midY + 104, {
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
    drawText(ctx, '未获取出口 IP', lx, midY + 72, { font: font(15), color: C.slate400 });
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
  lowerExitSubSuffix?: string;
  lowerHalted?: boolean;
  lowerLeftBadge: string;
  lowerRightBadge?: string;
  rightColor: string;
  rightTitle: string;
  rightSub: string;
}

/** 绘制共用的 4 节点 2.5D 对称菱形拓扑面板 */
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
  const ry = y + 66;
  const ey = y + h - 68;

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

  // 1. 你的浏览器
  ctx.fillStyle = 'rgba(0,0,0,0.48)';
  ctx.beginPath();
  ctx.ellipse(bx, cy + 18, 30, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  drawPolygon(ctx, [[bx - 24, cy + 6], [bx, cy - 6], [bx + 26, cy + 6], [bx + 2, cy + 18]], '#293548', '#475569');
  drawPolygon(ctx, [[bx - 24, cy + 6], [bx + 2, cy + 18], [bx + 2, cy + 22], [bx - 24, cy + 10]], '#141e30', '#334155');
  drawPolygon(ctx, [[bx - 24, cy + 6], [bx - 24, cy - 20], [bx, cy - 32], [bx, cy - 6]], '#0f172a', '#64748b');
  drawPolygon(ctx, [[bx - 21, cy + 3], [bx - 21, cy - 17], [bx - 3, cy - 26], [bx - 3, cy - 6]], '#0f2942');
  drawText(ctx, '你的浏览器', bx, cy + 38, { font: font(12, 600), color: C.slate200, align: 'center' });

  // 2. 上路信号节点
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
  drawText(ctx, truncate(ctx, spec.upperTitle, 240), mx, ry - 46, {
    font: font(12.5, 600),
    color: spec.upperTitleColor,
    align: 'center',
  });
  const upperSubIsIP = /^(?:\d{1,3}\.){2}|^[0-9a-fA-F]{1,4}:/.test(spec.upperSub);
  const upperSubFont = font(11, 400, upperSubIsIP ? MONO : SANS);
  ctx.font = upperSubFont;
  drawText(ctx, truncate(ctx, spec.upperSub, 240), mx, ry - 30, {
    font: upperSubFont,
    color: spec.upperSubColor,
    align: 'center',
  });

  // 3. 下路网页出口服务器
  ctx.fillStyle = 'rgba(74, 222, 128, 0.14)';
  ctx.beginPath();
  ctx.ellipse(mx, ey + 20, 28, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  drawPolygon(ctx, [[mx, ey - 26], [mx + 21, ey - 16], [mx, ey - 6], [mx - 21, ey - 16]], '#12402b', C.sig);
  drawPolygon(ctx, [[mx - 21, ey - 16], [mx, ey - 6], [mx, ey + 18], [mx - 21, ey + 8]], '#182234', '#334155');
  drawPolygon(ctx, [[mx, ey - 6], [mx + 21, ey - 16], [mx + 21, ey + 8], [mx, ey + 18]], '#0b1220', '#1e293b');
  ctx.font = font(12.5, 600);
  drawText(ctx, truncate(ctx, spec.lowerExitLabel, 240), mx, ey + 34, {
    font: font(12.5, 600),
    color: C.white,
    align: 'center',
  });
  drawText(ctx, `${spec.lowerExitIP} · ${spec.lowerExitSubSuffix ?? '网页出口'}`, mx, ey + 50, {
    font: font(11, 400, MONO),
    color: C.slate400,
    align: 'center',
  });

  // 4. 右侧汇聚判定塔
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

  drawText(ctx, '本次检测', PAD + 28, cardY + 24, { font: font(12.5, 500), color: C.slate400 });
  dot(ctx, PAD + 34, cardY + 52, color, soft, 5);
  drawText(ctx, title, PAD + 50, cardY + 52, { font: font(22, 700), color: C.white });
  ctx.font = font(14);
  drawText(ctx, truncate(ctx, maskIPsInText(note), cardW - 56), PAD + 28, cardY + 82, {
    font: font(14),
    color: C.slate300,
  });
}

async function toPngBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('canvas.toBlob 返回空'))), 'image/png');
  });
}

export async function saveShareImage(blob: Blob, prefix: string): Promise<string> {
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
    /* 剪贴板权限未开 */
  }
  return clipboardOk ? '分享图已复制并弹出下载' : '已弹出下载窗口（剪贴板权限未开）';
}

// =========================================================================
// DNS 分享卡
// =========================================================================

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
}

const DNS_VERDICT_PALETTE: Record<DNSShareCardInput['verdictState'], { color: string; soft: string; glow: string }> = {
  suspected: { color: C.leak, soft: C.leakSoft, glow: 'rgba(248, 113, 113, 0.16)' },
  clear: { color: C.sig, soft: C.sigSoft, glow: 'rgba(74, 222, 128, 0.14)' },
  unknown: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
  unobserved: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
};

export async function renderDNSShareCard(data: DNSShareCardInput): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;

  const canvas = document.createElement('canvas');
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);
  drawHeader(ctx, 'DNS 泄露检测报告', data.time);

  const pal = DNS_VERDICT_PALETTE[data.verdictState];
  const unobserved = data.verdictState === 'unobserved';
  const resNames = data.verdictState === 'suspected'
    ? data.providers.filter((p) => p.suspect).map((p) => p.name)
    : data.providers.map((p) => p.name);
  const resUnique = [...new Set((resNames.length ? resNames : data.providers.map((p) => p.name)).filter(Boolean))];
  const upperTitle = unobserved
    ? '权威 NS 未收到查询'
    : resUnique.length
      ? `${resUnique.slice(0, 2).join('、')}${resUnique.length > 2 ? ' 等' : ''}`
      : '归属信息不足';
  const upperSub = data.verdictState === 'suspected'
    ? '命中境内运营商 DNS'
    : data.verdictState === 'clear'
      ? `已采样 ${data.resolverCount} 个地址`
      : unobserved
        ? '请求被拦截或超时'
        : `${data.resolverCount} 个地址`;

  drawFlowDiamondStage(ctx, PAD, 102, W - PAD * 2, 250, {
    upperColor: pal.color,
    upperTopFill: data.verdictState === 'suspected' ? '#5c1922' : data.verdictState === 'clear' ? '#12402b' : '#293548',
    upperGlow: pal.glow,
    upperHalted: unobserved,
    upperTitle,
    upperTitleColor: data.verdictState === 'suspected' ? '#fca5a5' : C.white,
    upperSub,
    upperSubColor: data.verdictState === 'suspected' ? C.leak : C.slate400,
    upperLeftBadge: 'DNS 递归',
    upperRightBadge: unobserved ? '无捕获' : '权威查询',
    lowerExitLabel: data.exit?.ip ? `${data.exitNetworkName} · ${data.exitRegion}` : '未获取出口 IP',
    lowerExitIP: data.exit?.ip ? maskIP(data.exit.ip) : '—',
    lowerLeftBadge: 'HTTPS',
    lowerRightBadge: 'API',
    rightColor: pal.color,
    rightTitle: 'DetectRadar 服务端',
    rightSub: data.verdictTitle,
  });

  const cardY = 368;
  const cardW = W - PAD * 2;
  const cardH = H - cardY - PAD - 28;
  panel(ctx, PAD, cardY, cardW, cardH);
  drawVerdictBand(ctx, cardY, cardW, pal.color, pal.soft, data.verdictTitle, data.note);

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

  drawExitColumn(ctx, PAD + 28, midY, data.exit, data.exitNetworkName, data.exitRegion, data.exitTypeLabel);

  const rx = splitX + 28;
  const rightW = PAD + cardW - 28 - rx;
  drawText(ctx, 'DNS', rx, midY + 28, { font: font(13, 500), color: C.slate400 });
  drawText(ctx, `${data.resolverCount} 个地址`, rx + rightW, midY + 28, {
    font: font(12.5, 400, MONO),
    color: C.slate400,
    align: 'right',
  });

  if (data.providers.length === 0) {
    drawText(ctx, '未收到 DNS 解析记录', rx, midY + 72, { font: font(14), color: C.slate400 });
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
      const tag = p.type === 'public' ? '公共 DNS' : p.type === 'access' ? '运营商 DNS' : '';
      const tagW = tag ? ctx.measureText(tag).width + 24 : 0;
      ctx.font = font(15, 600);
      const nameStr = truncate(ctx, p.name, rightW - (nx - rx) - tagW - 80);
      drawText(ctx, nameStr, nx, rowY, { font: font(15, 600), color: p.suspect ? C.leak : C.slate200 });
      const nw = ctx.measureText(nameStr).width;
      if (p.regions) {
        drawText(ctx, p.regions, nx + nw + 10, rowY, { font: font(13), color: C.slate400 });
      }
      if (tag) {
        ctx.font = font(11.5, 500, MONO);
        const cw = ctx.measureText(tag).width + 16;
        chip(ctx, rx + rightW - cw, rowY, tag, C.slate300, 'rgba(148,163,184,0.08)', C.line);
      }
    });
    if (data.providers.length > shown.length) {
      drawText(ctx, `… 另有 ${data.providers.length - shown.length} 家解析服务商`, rx, cardY + cardH - 18, {
        font: font(12),
        color: C.slate500,
      });
    }
  }

  drawFooter(ctx, 'detectradar.com/dns-leak-test/');
  return toPngBlob(canvas);
}

// =========================================================================
// WebRTC 分享卡（同步 4 节点 2.5D 菱形路径 + 全量 IP 脱敏）
// =========================================================================

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
}

const WEBRTC_VERDICT_PALETTE: Record<WebRTCVerdictState, { color: string; soft: string; glow: string }> = {
  leaked: { color: C.leak, soft: C.leakSoft, glow: 'rgba(248, 113, 113, 0.16)' },
  clear: { color: C.sig, soft: C.sigSoft, glow: 'rgba(74, 222, 128, 0.14)' },
  unknown: { color: C.unknown, soft: C.unknownSoft, glow: 'rgba(148, 163, 184, 0.12)' },
};

function webrtcUpperCopy(data: WebRTCShareCardInput): { title: string; sub: string } {
  switch (data.verdictReason) {
    case 'disabled':
      return { title: 'WebRTC 已禁用', sub: '未暴露任何地址' };
    case 'no-candidates':
      return { title: '未测到 WebRTC 地址', sub: '未返回公网或内网 IP' };
    case 'mdns-protected':
      return { title: 'mDNS (*.local)', sub: `已隐藏内网 IP · ${data.mdnsCount} 个` };
    case 'local-exposed':
      return {
        title: 'WebRTC 内网地址',
        sub: data.localIPs[0] ? `${maskIP(data.localIPs[0].ip)} · 本机内网` : '仅读取到本机内网',
      };
    case 'public-leak':
    case 'proxy-bypass':
    case 'public-mismatch': {
      const ip = data.suspectIPs[0] ?? data.publicIPs[0];
      return {
        title: 'WebRTC 公网 IP',
        sub: ip ? `${maskIP(ip)} · 与出口不同` : '与出口不同',
      };
    }
    case 'dual-stack':
      return {
        title: 'WebRTC 公网 IP',
        sub: data.publicIPs[0] ? `${maskIP(data.publicIPs[0])} · 同运营商` : '同运营商 IPv4/IPv6',
      };
    case 'same-prefix':
      return {
        title: 'WebRTC IPv6 地址',
        sub: data.publicIPs[0] ? `${maskIP(data.publicIPs[0])} · 同网段 IPv6` : '同网段 IPv6',
      };
    case 'same-network':
      return {
        title: 'WebRTC 同网出口',
        sub: data.publicIPs[0] ? `${maskIP(data.publicIPs[0])} · 共享出口` : '同运营商共享出口',
      };
    case 'assessment-unavailable':
      return {
        title: 'WebRTC 公网 IP',
        sub: data.publicIPs[0] ? `${maskIP(data.publicIPs[0])} · 归属待核对` : '暂未完成归属核对',
      };
    case 'missing-exit':
      return {
        title: 'WebRTC 公网 IP',
        sub: data.publicIPs[0] ? maskIP(data.publicIPs[0]) : '未获取出口 IP',
      };
    case 'matches-exit':
    default:
      return {
        title: 'WebRTC 公网 IP',
        sub: data.publicIPs[0] ? `${maskIP(data.publicIPs[0])} · 与出口一致` : '与出口一致',
      };
  }
}

export async function renderWebRTCShareCard(data: WebRTCShareCardInput): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;

  const canvas = document.createElement('canvas');
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);
  drawHeader(ctx, 'WebRTC 泄露检测报告', data.time);

  const pal = WEBRTC_VERDICT_PALETTE[data.verdictState];
  const halted = !data.apiSupported || data.verdictReason === 'no-candidates';
  const upperColor = halted ? C.unknown : pal.color;
  const upperTopFill =
    data.verdictState === 'leaked'
      ? '#5c1922'
      : !halted && data.verdictState === 'clear'
        ? '#12402b'
        : '#293548';
  const upperCopy = webrtcUpperCopy(data);

  drawFlowDiamondStage(ctx, PAD, 102, W - PAD * 2, 250, {
    upperColor,
    upperTopFill,
    upperGlow: halted ? 'rgba(148, 163, 184, 0.12)' : pal.glow,
    upperHalted: halted,
    upperTitle: upperCopy.title,
    upperTitleColor: data.verdictState === 'leaked' ? '#fca5a5' : C.white,
    upperSub: upperCopy.sub,
    upperSubColor: data.verdictState === 'leaked' ? C.leak : C.slate400,
    upperLeftBadge: 'STUN 探测',
    upperRightBadge: halted ? '未返回' : '本地读取',
    lowerExitLabel: data.exit?.ip ? `${data.exitNetworkName} · ${data.exitRegion}` : '未获取出口 IP',
    lowerExitIP: data.exit?.ip ? maskIP(data.exit.ip) : '—',
    lowerLeftBadge: 'HTTPS',
    lowerRightBadge: 'API',
    rightColor: pal.color,
    rightTitle: '出口归属比对',
    rightSub: data.verdictTitle,
  });

  const cardY = 368;
  const cardW = W - PAD * 2;
  const cardH = H - cardY - PAD - 28;
  panel(ctx, PAD, cardY, cardW, cardH);
  drawVerdictBand(ctx, cardY, cardW, pal.color, pal.soft, data.verdictTitle, data.verdictNote);

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
  drawExitColumn(ctx, lx, midY, data.exit, data.exitNetworkName, data.exitRegion, data.exitTypeLabel);

  ctx.strokeStyle = C.line;
  ctx.beginPath();
  ctx.moveTo(lx, midY + 180);
  ctx.lineTo(splitX - 28, midY + 180);
  ctx.stroke();

  const stats: [string, string][] = [
    ['WebRTC 公网 IP', `${data.publicIPs.length} 个`],
    ['本机内网地址', `${data.localIPs.length} 个`],
    ['mDNS 隐藏地址', `${data.mdnsCount} 个`],
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
  drawText(ctx, '公网 IP', rx, r1Y, { font: font(14, 500), color: C.slate200 });
  if (!data.apiSupported) {
    chip(ctx, rx + rightW - 96, r1Y, 'WebRTC 已禁用', C.slate300, 'rgba(148,163,184,0.08)', C.line);
  } else if (data.publicIPs.length === 0) {
    drawText(ctx, '未测到公网 IP', rx + rightW, r1Y, { font: font(13), color: C.slate400, align: 'right' });
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
        ? '与出口不同'
        : matchesExit
          ? '与出口一致'
          : samePrefix
            ? '同网段 IPv6'
            : data.verdictReason === 'dual-stack'
              ? '同运营商'
              : data.verdictReason === 'same-network'
                ? '共享出口'
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
  drawText(ctx, '内网地址', rx, r2Y, { font: font(14, 500), color: C.slate200 });
  if (!data.apiSupported) {
    drawText(ctx, '接口已禁用', rx + rightW, r2Y, { font: font(13), color: C.slate400, align: 'right' });
  } else if (data.localIPs.length === 0 && data.mdnsCount > 0) {
    const label = 'mDNS 已隐藏（*.local）';
    ctx.font = font(11.5, 500, MONO);
    const cw = ctx.measureText(label).width + 16;
    chip(ctx, rx + rightW - cw, r2Y, label, C.sig, C.sigSoft, 'rgba(74,222,128,0.35)');
  } else if (data.localIPs.length === 0) {
    drawText(ctx, '未暴露', rx + rightW, r2Y, { font: font(13), color: C.slate400, align: 'right' });
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
  drawText(ctx, 'IPv6 地址', rx, r3Y, { font: font(14, 500), color: C.slate200 });
  if (data.ipv6Global) {
    drawText(ctx, maskIP(data.ipv6Global), rx + rightW, r3Y, {
      font: font(14, 500, MONO),
      color: C.slate200,
      align: 'right',
    });
  } else {
    drawText(ctx, data.apiSupported ? '未测到全局 IPv6' : '禁用 WebRTC 时跳过', rx + rightW, r3Y, {
      font: font(13),
      color: C.slate400,
      align: 'right',
    });
  }

  drawFooter(ctx, 'detectradar.com/webrtc-leak-test/');
  return toPngBlob(canvas);
}

// =========================================================================
// IPv6 双栈分享卡（4 节点 2.5D 菱形路径 + 全量 IP 脱敏）
// =========================================================================

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
  heading: string,
  exit: StackExit | null,
  networkName: string,
  region: string,
  typeLabel: string,
  opts: { accent?: string; statusChip?: { label: string; color: string; soft: string; border: string }; emptyText: string },
) {
  drawText(ctx, heading, x, midY + 28, { font: font(13, 500), color: C.slate400 });
  if (exit?.ip) {
    drawText(ctx, `${networkName}  ${region}`, x, midY + 62, {
      font: font(16, 600),
      color: C.slate200,
    });
    drawText(ctx, maskIP(exit.ip), x, midY + 104, {
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
      drawText(ctx, `探测耗时 ${exit.latencyMs} ms`, x, midY + 186, {
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

  const canvas = document.createElement('canvas');
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);
  drawHeader(ctx, 'IPv6 泄露检测报告', data.time);

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
    : 'IPv6 未连通';
  const upperSub = data.v6?.ip
    ? `${maskIP(data.v6.ip)}${data.verdictState === 'leaked' ? ' · 旁路直连' : ''}`
    : '仅连通 IPv4 网络';

  drawFlowDiamondStage(ctx, PAD, 102, W - PAD * 2, 250, {
    upperColor,
    upperTopFill,
    upperGlow: v6Halted ? 'rgba(148, 163, 184, 0.12)' : data.verdictState === 'leaked' ? C.leakSoft : C.sigSoft,
    upperHalted: v6Halted,
    upperTitle,
    upperTitleColor: data.verdictState === 'leaked' ? '#fca5a5' : C.white,
    upperSub,
    upperSubColor: data.verdictState === 'leaked' ? C.leak : C.slate400,
    upperLeftBadge: 'IPv6 探测',
    upperRightBadge: v6Halted ? '未连通' : '6.detectradar',
    lowerExitLabel: data.v4?.ip ? `${data.v4NetworkName} · ${data.v4Region}` : 'IPv4 未连通',
    lowerExitIP: data.v4?.ip ? maskIP(data.v4.ip) : '—',
    lowerExitSubSuffix: 'IPv4 出口',
    lowerHalted: !data.v4?.ip,
    lowerLeftBadge: 'IPv4 探测',
    lowerRightBadge: data.v4?.ip ? '4.detectradar' : '未连通',
    rightColor: pal.color,
    rightTitle: '双栈归属比对',
    rightSub: data.verdictTitle,
  });

  const cardY = 368;
  const cardW = W - PAD * 2;
  const cardH = H - cardY - PAD - 28;
  panel(ctx, PAD, cardY, cardW, cardH);
  drawVerdictBand(ctx, cardY, cardW, pal.color, pal.soft, data.verdictTitle, data.verdictNote);

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

  drawStackColumn(
    ctx,
    PAD + 28,
    midY,
    'IPv4 出口',
    data.v4,
    data.v4NetworkName,
    data.v4Region,
    data.v4TypeLabel,
    { emptyText: '未连通 IPv4' },
  );

  const v6Chip = data.v6?.ip
    ? data.verdictState === 'leaked'
      ? { label: '境内直连', color: C.leak, soft: C.leakSoft, border: 'rgba(248,113,113,0.35)' }
      : data.verdictReason === 'overseas-split'
        ? { label: '双栈分流', color: C.sig, soft: C.sigSoft, border: 'rgba(74,222,128,0.35)' }
        : undefined
    : undefined;

  drawStackColumn(
    ctx,
    splitX + 28,
    midY,
    'IPv6 出口',
    data.v6,
    data.v6NetworkName,
    data.v6Region,
    data.v6TypeLabel,
    {
      accent: data.verdictState === 'leaked' ? C.leak : C.white,
      statusChip: v6Chip,
      emptyText: '未连通 IPv6（仅 IPv4 网络）',
    },
  );

  drawFooter(ctx, 'detectradar.com/ipv6-leak-test/');
  return toPngBlob(canvas);
}
