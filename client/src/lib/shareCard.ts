/**
 * 分享卡渲染器：用 Canvas 2D 从 ReportModel 直接绘制固定构图的报告分享图。
 *
 * 为什么不用 DOM 截图（html-to-image/snapdom/html2canvas）：这类库都是
 * 「克隆 DOM → 序列化 → 让浏览器重新排版渲染」，重排环境与真实页面存在
 * 系统性偏差（动画时间线、backdrop-filter、字体回退、浏览器缩放、DPR、
 * 伪类状态……），修不完。Canvas 直绘每个像素都由我们控制，与页面布局
 * 和用户环境彻底解耦。
 */

import type { ReportModel, SignalState, Severity } from "./report";

// 逻辑画布尺寸（输出 2x = 2880×2160）
const W = 1440;
const H = 1080;
const SCALE = 2;
const PAD = 56;

// 与 global.css :root 对齐的调色板（canvas 不解析 var()，这里静态镜像）
const C = {
  bg: "#020617",
  panel: "rgba(30, 41, 59, 0.45)",
  line: "rgba(255, 255, 255, 0.08)",
  grid: "rgba(34, 197, 94, 0.03)",
  radar: "#4ade80",
  white: "#ffffff",
  slate200: "#e2e8f0",
  slate300: "#cbd5e1",
  slate400: "#94a3b8",
  slate500: "#64748b",
  slate600: "#475569",
  sig: "#4ade80",
  warn: "#fbbf24",
  leak: "#f87171",
  unknown: "#94a3b8",
  sigSoft: "rgba(74, 222, 128, 0.12)",
  warnSoft: "rgba(251, 191, 36, 0.12)",
  leakSoft: "rgba(248, 113, 113, 0.13)",
  unknownSoft: "rgba(148, 163, 184, 0.1)",
};

const STATE_COLOR: Record<SignalState, string> = {
  coherent: C.sig,
  contradiction: C.warn,
  leak: C.leak,
  unknown: C.unknown,
};
const STATE_SOFT: Record<SignalState, string> = {
  coherent: C.sigSoft,
  contradiction: C.warnSoft,
  leak: C.leakSoft,
  unknown: C.unknownSoft,
};

const SEV_META: Record<
  Severity,
  { color: string; soft: string; label: string }
> = {
  leak: { color: C.leak, soft: C.leakSoft, label: "暴露" },
  contradiction: { color: C.warn, soft: C.warnSoft, label: "冲突" },
  caution: { color: C.warn, soft: C.warnSoft, label: "留意" },
  info: { color: C.unknown, soft: C.unknownSoft, label: "提示" },
};

// 与页面同源的字体栈；CJK 显式列出保证跨平台稳定
const SANS = `Inter, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif`;
const MONO = `"JetBrains Mono", ui-monospace, Menlo, monospace`;

function font(size: number, weight = 400, family = SANS): string {
  return `${weight} ${size}px ${family}`;
}

/** 文本超宽时截断加省略号 */
function truncate(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxW: number,
): string {
  if (ctx.measureText(text).width <= maxW) return text;
  let t = text;
  while (t.length > 1 && ctx.measureText(t + "…").width > maxW)
    t = t.slice(0, -1);
  return t + "…";
}

function panel(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
) {
  ctx.fillStyle = C.panel;
  ctx.strokeStyle = C.line;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, 14);
  ctx.fill();
  ctx.stroke();
}

function dot(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  color: string,
  soft: string,
) {
  ctx.fillStyle = soft;
  ctx.beginPath();
  ctx.arc(x, y, 7, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.arc(x, y, 4, 0, Math.PI * 2);
  ctx.fill();
}

/** 圆角小徽章（severity 章 / IP 类型章），返回占用宽度 */
function chip(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  text: string,
  color: string,
  soft: string,
  size = 12,
): number {
  ctx.font = font(size, 500, MONO);
  const w = ctx.measureText(text).width + 18;
  const h = size + 12;
  ctx.fillStyle = soft;
  ctx.beginPath();
  ctx.roundRect(x, y - h / 2, w, h, 6);
  ctx.fill();
  ctx.fillStyle = color;
  ctx.textAlign = "left";
  ctx.textBaseline = "middle";
  ctx.fillText(text, x + 9, y + 1);
  return w;
}

function text(
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
  ctx.textAlign = opts.align ?? "left";
  ctx.textBaseline = opts.baseline ?? "middle";
  ctx.fillText(s, x, y);
}

// ---- 背景：深空底 + 扫描网格 + 角落光晕 ----

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

  const glow1 = ctx.createRadialGradient(0, 0, 0, 0, 0, 620);
  glow1.addColorStop(0, "rgba(34, 197, 94, 0.10)");
  glow1.addColorStop(1, "rgba(34, 197, 94, 0)");
  ctx.fillStyle = glow1;
  ctx.fillRect(0, 0, W, H);

  const glow2 = ctx.createRadialGradient(W, H, 0, W, H, 700);
  glow2.addColorStop(0, "rgba(37, 99, 235, 0.10)");
  glow2.addColorStop(1, "rgba(37, 99, 235, 0)");
  ctx.fillStyle = glow2;
  ctx.fillRect(0, 0, W, H);
}

// ---- 雷达图（与 RadarCard 同一套轴角/顶点数学）----

function drawRadar(
  ctx: CanvasRenderingContext2D,
  report: ReportModel,
  cx: number,
  cy: number,
  R: number,
) {
  const groups = report.groups;
  const angle = (i: number) => ((-90 + i * 90) * Math.PI) / 180;
  const pt = (i: number, r: number) => ({
    x: cx + r * Math.cos(angle(i)),
    y: cy + r * Math.sin(angle(i)),
  });

  ctx.strokeStyle = C.line;
  ctx.lineWidth = 1;
  for (const f of [0.33, 0.66, 1]) {
    ctx.beginPath();
    ctx.arc(cx, cy, R * f, 0, Math.PI * 2);
    ctx.stroke();
  }
  for (let i = 0; i < groups.length; i++) {
    const e = pt(i, R);
    ctx.beginPath();
    ctx.moveTo(cx, cy);
    ctx.lineTo(e.x, e.y);
    ctx.stroke();
  }

  const verdictColor = STATE_COLOR[report.verdict.state];
  const verts = groups.map((g, i) => pt(i, (Math.max(5, g.score) / 100) * R));
  ctx.beginPath();
  verts.forEach((v, i) =>
    i === 0 ? ctx.moveTo(v.x, v.y) : ctx.lineTo(v.x, v.y),
  );
  ctx.closePath();
  ctx.globalAlpha = 0.16;
  ctx.fillStyle = verdictColor;
  ctx.fill();
  ctx.globalAlpha = 1;
  ctx.strokeStyle = verdictColor;
  ctx.lineWidth = 1.5;
  ctx.lineJoin = "round";
  ctx.stroke();

  verts.forEach((v, i) => {
    ctx.fillStyle = STATE_COLOR[groups[i].state];
    ctx.strokeStyle = C.bg;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(v.x, v.y, 3.5, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
  });

  // 轴标签 + 分数
  const labelPos = groups.map((_, i) => pt(i, R + 30));
  const anchors: CanvasTextAlign[] = ["center", "left", "center", "right"];
  groups.forEach((g, i) => {
    const p = labelPos[i];
    text(ctx, g.label, p.x, p.y - 7, {
      font: font(12, 400, MONO),
      color: C.slate400,
      align: anchors[i],
    });
    text(ctx, String(g.score), p.x, p.y + 10, {
      font: font(11, 500, MONO),
      color: STATE_COLOR[g.state],
      align: anchors[i],
    });
  });
}

// ---- 主入口 ----

export interface ShareCardOptions {
  riskLabel: string;
}

export async function renderShareCard(
  report: ReportModel,
  opts: ShareCardOptions,
): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;

  const canvas = document.createElement("canvas");
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext("2d")!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);

  // == 页眉 ==
  let y = PAD + 8;
  text(ctx, "Detect", PAD, y, { font: font(28, 700, MONO), color: C.white });
  const dw = ctx.measureText("Detect").width;
  text(ctx, "Radar", PAD + dw, y, {
    font: font(28, 700, MONO),
    color: C.radar,
  });
  text(ctx, "网络环境一致性检测报告", W - PAD, y - 10, {
    font: font(14),
    color: C.slate400,
    align: "right",
  });
  const when = report.meta.processedAt
    ? new Date(report.meta.processedAt)
    : new Date();
  const stamp = `${when.getFullYear()}-${String(when.getMonth() + 1).padStart(2, "0")}-${String(when.getDate()).padStart(2, "0")} ${String(when.getHours()).padStart(2, "0")}:${String(when.getMinutes()).padStart(2, "0")}`;
  text(
    ctx,
    `${stamp} · scan ${report.meta.scanId.slice(0, 8)}`,
    W - PAD,
    y + 12,
    {
      font: font(12, 400, MONO),
      color: C.slate600,
      align: "right",
    },
  );

  // == 主区：左雷达 / 右判定+破绽 ==
  const topY = y + 44;
  const leftW = 430;
  const rightX = PAD + leftW + 20;
  const rightW = W - PAD - rightX;
  const mainH = 462;

  panel(ctx, PAD, topY, leftW, mainH);
  drawRadar(ctx, report, PAD + leftW / 2, topY + 196, 128);
  text(ctx, "综合风险", PAD + leftW / 2 - 14, topY + mainH - 56, {
    font: font(13),
    color: C.slate500,
    align: "right",
  });
  text(ctx, opts.riskLabel, PAD + leftW / 2 - 2, topY + mainH - 56, {
    font: font(15, 600),
    color: STATE_COLOR[report.verdict.state],
    align: "left",
  });

  panel(ctx, rightX, topY, rightW, mainH);
  const vx = rightX + 28;
  dot(
    ctx,
    vx + 4,
    topY + 44,
    STATE_COLOR[report.verdict.state],
    STATE_SOFT[report.verdict.state],
  );
  text(ctx, report.verdict.headline, vx + 22, topY + 44, {
    font: font(24, 700),
    color: C.white,
  });
  ctx.font = font(14);
  text(
    ctx,
    truncate(ctx, report.verdict.sub, rightW - 80),
    vx + 22,
    topY + 74,
    {
      font: font(14),
      color: C.slate400,
    },
  );

  // 分隔线 + 破绽清单
  ctx.strokeStyle = C.line;
  ctx.beginPath();
  ctx.moveTo(vx, topY + 100);
  ctx.lineTo(rightX + rightW - 28, topY + 100);
  ctx.stroke();

  const findings = report.findings.slice(0, 5);
  if (findings.length === 0) {
    dot(ctx, vx + 4, topY + 136, C.sig, C.sigSoft);
    text(ctx, "未见明显异常", vx + 22, topY + 136, {
      font: font(15),
      color: C.slate200,
    });
  } else {
    text(ctx, "暴露项", vx, topY + 124, {
      font: font(12, 400, MONO),
      color: C.slate500,
    });
    text(
      ctx,
      String(report.findings.length),
      rightX + rightW - 28,
      topY + 124,
      {
        font: font(12, 400, MONO),
        color: C.slate600,
        align: "right",
      },
    );
    findings.forEach((f, i) => {
      const fy = topY + 158 + i * 58;
      const m = SEV_META[f.severity];
      const cw = chip(ctx, vx, fy, m.label, m.color, m.soft);
      ctx.font = font(15, 500);
      text(
        ctx,
        truncate(ctx, f.title, rightW - cw - 76),
        vx + cw + 14,
        fy - 1,
        {
          font: font(15, 500),
          color: C.slate200,
        },
      );
      ctx.font = font(12);
      text(
        ctx,
        truncate(ctx, f.fact, rightW - cw - 76),
        vx + cw + 14,
        fy + 21,
        {
          font: font(12),
          color: C.slate500,
        },
      );
    });
    if (report.findings.length > findings.length) {
      text(
        ctx,
        `… 另有 ${report.findings.length - findings.length} 项，详见完整报告`,
        vx,
        topY + mainH - 26,
        {
          font: font(12),
          color: C.slate600,
        },
      );
    }
  }

  // == 身份条 ==
  // 带 RTT 读数时加高一档，底部放「物理延迟」读数行（与页面 IdentityCard 同构）
  const idY = topY + mainH + 20;
  const rtt = report.rtt;
  const idH = rtt ? 152 : 118;
  panel(ctx, PAD, idY, W - PAD * 2, idH);
  const idCy = idY + (rtt ? 46 : idH / 2); // 上排（IP + 四列读数）的纵向中心
  let ix = PAD + 28;
  text(ctx, "出口 IP", ix, idCy - 25, { font: font(12), color: C.slate500 });
  text(ctx, report.identity.masked || "—", ix, idCy + 7, {
    font: font(26, 600, MONO),
    color: C.white,
  });
  ctx.font = font(26, 600, MONO);
  const ipW = Math.max(
    ctx.measureText(report.identity.masked || "—").width,
    220,
  );
  chip(
    ctx,
    ix + ipW + 20,
    idCy + 3,
    report.identity.usageLabel,
    STATE_COLOR[report.identity.state],
    STATE_SOFT[report.identity.state],
    13,
  );

  // 右侧四列读数
  const cols = [
    {
      label: "归属地",
      value:
        (report.identity.countryCode
          ? `[${report.identity.countryCode}] `
          : "") + report.identity.geo,
      color: C.slate200,
    },
    { label: "运营商", value: report.identity.org, color: C.slate200 },
    {
      label: "欺诈评分",
      // 档位词与页面 IdentityCard 同一分档（很低/较低/偏高/高危）
      value: `${report.identity.fraudScore} / 100（${
        report.identity.fraudScore < 25
          ? "很低"
          : report.identity.fraudScore < 50
            ? "较低"
            : report.identity.fraudScore < 75
              ? "偏高"
              : "高危"
      }）`,
      color:
        report.identity.fraudScore >= 50
          ? C.leak
          : report.identity.fraudScore >= 25
            ? C.warn
            : C.sig,
    },
    {
      label: "黑名单",
      value: report.identity.blacklist.length
        ? `命中 ${report.identity.blacklist.length}`
        : "未命中",
      color: report.identity.blacklist.length ? C.leak : C.sig,
    },
  ];
  // 归属地/运营商内容长，按权重分列宽
  const colsX = PAD + 480;
  const colsTotal = W - PAD - 28 - colsX;
  const weights = [1.5, 1.1, 0.7, 0.7];
  const weightSum = weights.reduce((a, b) => a + b, 0);
  let cx = colsX;
  cols.forEach((c, i) => {
    const colW = (colsTotal * weights[i]) / weightSum;
    text(ctx, c.label, cx, idCy - 16, { font: font(12), color: C.slate500 });
    ctx.font = font(14, 500);
    text(ctx, truncate(ctx, c.value, colW - 18), cx, idCy + 12, {
      font: font(14, 500),
      color: c.color,
    });
    cx += colW;
  });

  // 物理延迟读数行（Phase 1）：与页面同一形态——纯测量值、全灰阶、明标不参与评分
  if (rtt) {
    ctx.strokeStyle = C.line;
    ctx.beginPath();
    ctx.moveTo(PAD + 28, idY + 92);
    ctx.lineTo(W - PAD - 28, idY + 92);
    ctx.stroke();

    const ly = idY + 122;
    let rx = PAD + 28;
    ctx.font = font(12, 400, MONO);
    text(ctx, "物理延迟", rx, ly, {
      font: font(12, 400, MONO),
      color: C.slate500,
    });
    rx += ctx.measureText("物理延迟").width + 30;

    const item = (label: string, v: number) => {
      ctx.font = font(13);
      text(ctx, label, rx, ly, { font: font(13), color: C.slate500 });
      rx += ctx.measureText(label).width + 9;
      const val = v.toFixed(1);
      ctx.font = font(15, 500, MONO);
      text(ctx, val, rx, ly, {
        font: font(15, 500, MONO),
        color: C.slate200,
      });
      rx += ctx.measureText(val).width + 5;
      ctx.font = font(11);
      text(ctx, "ms", rx, ly, { font: font(11), color: C.slate600 });
      rx += ctx.measureText("ms").width + 34;
    };
    if (rtt.clientMinMS != null) item("浏览器 ↔ 服务器", rtt.clientMinMS);
    if (rtt.serverTCPMS != null) item("出口 ↔ 服务器", rtt.serverTCPMS);
    if (rtt.deltaMS != null) item("Δ 浏览器 ↔ 出口", rtt.deltaMS);

    text(ctx, "试运行 · 不参与评分", W - PAD - 28, ly, {
      font: font(11),
      color: C.slate600,
      align: "right",
    });
  }

  // == 三组明细 ==
  const detY = idY + idH + 20;
  const detH = H - detY - PAD - 34;
  const detW = (W - PAD * 2 - 40) / 3;
  const sections = [
    {
      title: "环境一致性",
      rows: report.consistency,
      state: report.groups[1]?.state ?? "unknown",
    },
    {
      title: "DNS/IP 泄露",
      rows: report.leaks,
      state: report.groups[2]?.state ?? "unknown",
    },
    {
      title: "环境指纹",
      rows: report.environment,
      state: report.groups[3]?.state ?? "unknown",
    },
  ];
  sections.forEach((s, i) => {
    const sx = PAD + i * (detW + 20);
    panel(ctx, sx, detY, detW, detH);
    text(ctx, s.title, sx + 24, detY + 32, {
      font: font(14, 600),
      color: C.white,
    });
    dot(
      ctx,
      sx + detW - 26,
      detY + 32,
      STATE_COLOR[s.state],
      STATE_SOFT[s.state],
    );
    s.rows.slice(0, 5).forEach((r, j) => {
      const ry = detY + 68 + j * 33;
      dot(ctx, sx + 28, ry, STATE_COLOR[r.state], STATE_SOFT[r.state]);
      text(ctx, r.label, sx + 44, ry, { font: font(13), color: C.slate400 });
      ctx.font = font(13);
      text(ctx, truncate(ctx, r.value, detW - 130), sx + detW - 24, ry, {
        font: font(13),
        color: C.slate200,
        align: "right",
      });
    });
  });

  // == 页脚 ==
  text(ctx, "detectradar.com", PAD, H - PAD + 18, {
    font: font(13, 500, MONO),
    color: C.slate500,
  });
  text(ctx, "核对出口 IP 与浏览器环境 · 检查 WebRTC / DNS / IPv6 泄露", W - PAD, H - PAD + 18, {
    font: font(12),
    color: C.slate600,
    align: "right",
  });

  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("canvas.toBlob 返回空"))),
      "image/png",
    );
  });
}
