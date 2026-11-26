import { getMessages, type Locale } from "../i18n";
import { maskIPsInText } from "./ipDisplay";
import type { ReportModel, SignalState, Severity } from "./report";

const W = 1440;
const H = 1080;
const SCALE = 2;
const PAD = 56;

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

function sevMeta(locale: Locale): Record<
  Severity,
  { color: string; soft: string; label: string }
> {
  const s = getMessages(locale).home.diagnosis.sev;
  return {
    leak: { color: C.leak, soft: C.leakSoft, label: s.leak },
    contradiction: { color: C.warn, soft: C.warnSoft, label: s.contradiction },
    caution: { color: C.warn, soft: C.warnSoft, label: s.caution },
    info: { color: C.unknown, soft: C.unknownSoft, label: s.info },
  };
}

const SANS = `Inter, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif`;
const MONO = `"JetBrains Mono", ui-monospace, Menlo, monospace`;

function font(size: number, weight = 400, family = SANS): string {
  return `${weight} ${size}px ${family}`;
}

function truncate(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxW: number,
): string {
  if (maxW <= 12) return "…";
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

function drawRadar(
  ctx: CanvasRenderingContext2D,
  report: ReportModel,
  cx: number,
  cy: number,
  R: number,
  locale: Locale,
) {
  const shortLabels = getMessages(locale).home.radarShort;
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

  const labelOffset = locale === "en" ? 18 : 30;
  const labelPos = groups.map((_, i) => pt(i, R + labelOffset));
  const anchors: CanvasTextAlign[] = ["center", "left", "center", "right"];
  groups.forEach((g, i) => {
    const p = labelPos[i];
    text(ctx, shortLabels[g.key] ?? g.label, p.x, p.y - 7, {
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

export interface ShareCardOptions {
  riskLabel: string;
  locale?: Locale;
}

export async function renderShareCard(
  report: ReportModel,
  opts: ShareCardOptions,
): Promise<Blob> {
  if (document.fonts?.ready) await document.fonts.ready;

  const locale: Locale = opts.locale ?? "zh-CN";
  const m = getMessages(locale);
  const sc = m.home.shareCard;
  const idCopy = m.home.identity;
  const sevs = sevMeta(locale);

  const canvas = document.createElement("canvas");
  canvas.width = W * SCALE;
  canvas.height = H * SCALE;
  const ctx = canvas.getContext("2d")!;
  ctx.scale(SCALE, SCALE);

  drawBackground(ctx);

  let y = PAD + 8;
  text(ctx, "Detect", PAD, y, { font: font(28, 700, MONO), color: C.white });
  const dw = ctx.measureText("Detect").width;
  text(ctx, "Radar", PAD + dw, y, {
    font: font(28, 700, MONO),
    color: C.radar,
  });
  text(
    ctx,
    report.scenario === "daily" ? sc.subtitleDaily : sc.subtitleStrict,
    W - PAD,
    y - 10,
    {
      font: font(14),
      color: C.slate400,
      align: "right",
    },
  );
  const when = report.meta.processedAt
    ? new Date(report.meta.processedAt)
    : new Date();
  const stamp = `${when.getFullYear()}-${String(when.getMonth() + 1).padStart(2, "0")}-${String(when.getDate()).padStart(2, "0")} ${String(when.getHours()).padStart(2, "0")}:${String(when.getMinutes()).padStart(2, "0")}`;
  const rawTier = (report.meta.tier ?? "").trim();
  const tierLabel =
    rawTier && rawTier !== "—"
      ? rawTier.includes("L3")
        ? sc.tierDeep(rawTier)
        : sc.tierLocal(rawTier)
      : "";
  const metaLine = [
    stamp,
    tierLabel,
    `scan ${report.meta.scanId.slice(0, 8)}`,
  ]
    .filter(Boolean)
    .join(" · ");
  text(ctx, metaLine, W - PAD, y + 12, {
    font: font(12, 400, MONO),
    color: C.slate500,
    align: "right",
  });

  const topY = y + 44;
  const leftW = 430;
  const rightX = PAD + leftW + 20;
  const rightW = W - PAD - rightX;
  const mainH = 462;

  panel(ctx, PAD, topY, leftW, mainH);
  drawRadar(ctx, report, PAD + leftW / 2, topY + 196, locale === "en" ? 106 : 128, locale);
  text(ctx, m.home.overallRisk, PAD + leftW / 2 - 10, topY + mainH - 56, {
    font: font(13),
    color: C.slate500,
    align: "right",
  });
  text(ctx, opts.riskLabel, PAD + leftW / 2 + 2, topY + mainH - 56, {
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
  ctx.font = font(24, 700);
  text(ctx, truncate(ctx, report.verdict.headline, rightW - 80), vx + 22, topY + 44, {
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

  ctx.strokeStyle = C.line;
  ctx.beginPath();
  ctx.moveTo(vx, topY + 100);
  ctx.lineTo(rightX + rightW - 28, topY + 100);
  ctx.stroke();

  const findings = report.findings.slice(0, 5);
  if (findings.length === 0) {
    dot(ctx, vx + 4, topY + 136, C.sig, C.sigSoft);
    text(ctx, m.home.diagnosis.noIssues, vx + 22, topY + 136, {
      font: font(15),
      color: C.slate200,
    });
  } else {
    text(ctx, m.home.diagnosis.findingsLabel, vx, topY + 124, {
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
      const sm = sevs[f.severity];
      const cw = chip(ctx, vx, fy, sm.label, sm.color, sm.soft);
      ctx.font = font(15, 500);
      text(
        ctx,
        truncate(ctx, maskIPsInText(f.title), rightW - cw - 76),
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
        truncate(ctx, maskIPsInText(f.fact), rightW - cw - 76),
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
        sc.moreFindings(report.findings.length - findings.length),
        vx,
        topY + mainH - 26,
        {
          font: font(12),
          color: C.slate600,
        },
      );
    }
  }

  const idY = topY + mainH + 20;
  const rtt = report.rtt;
  const humanPct =
    typeof report.identity.humanRatio === "number" &&
    Number.isFinite(report.identity.humanRatio)
      ? Math.round(Math.max(0, Math.min(100, report.identity.humanRatio)) * 10) /
        10
      : null;
  const fmtPct = (x: number) => (Number.isInteger(x) ? String(x) : x.toFixed(1));
  const idH = rtt ? 152 : 118;
  panel(ctx, PAD, idY, W - PAD * 2, idH);
  const idCy = idY + (rtt ? 46 : idH / 2);
  let ix = PAD + 28;
  text(ctx, m.common.exitIP, ix, idCy - 25, { font: font(12), color: C.slate500 });
  ctx.font = font(26, 600, MONO);
  const ipMaxW = locale === "en" ? 270 : 250;
  const ipText = truncate(ctx, report.identity.masked || "—", ipMaxW);
  text(ctx, ipText, ix, idCy + 7, {
    font: font(26, 600, MONO),
    color: C.white,
  });
  ctx.font = font(26, 600, MONO);
  const ipW = Math.min(Math.max(ctx.measureText(ipText).width, 170), ipMaxW);
  const usageBadgeState: SignalState =
    report.identity.state === "coherent" &&
    report.identity.ipType === "datacenter"
      ? "unknown"
      : report.identity.state;
  chip(
    ctx,
    ix + ipW + 14,
    idCy + 3,
    report.identity.usageLabel,
    STATE_COLOR[usageBadgeState],
    STATE_SOFT[usageBadgeState],
    locale === "en" ? 11.5 : 13,
  );

  const rawAsn = (report.identity.asn ?? "").trim();
  const asnDisplay =
    !rawAsn || rawAsn === "—"
      ? "—"
      : /^as\d+$/i.test(rawAsn)
        ? rawAsn.toUpperCase()
        : /^\d+$/.test(rawAsn)
          ? `AS${rawAsn}`
          : rawAsn;
  const fraudTag =
    report.identity.fraudScore < 25
      ? idCopy.fraudLow
      : report.identity.fraudScore < 50
        ? idCopy.fraudModLow
        : report.identity.fraudScore < 75
          ? idCopy.fraudHigh
          : idCopy.fraudCritical;
  const humanTag =
    humanPct == null
      ? ""
      : humanPct >= 70
        ? idCopy.humanHigh
        : humanPct >= 40
          ? idCopy.humanMid
          : idCopy.humanLow;
  const lp = locale === "en" ? " (" : "（";
  const rp = locale === "en" ? ")" : "）";
  const cols: { label: string; value: string; color: string; mono?: boolean }[] = [
    {
      label: idCopy.location,
      value:
        (report.identity.countryCode
          ? `[${report.identity.countryCode}] `
          : "") + report.identity.geo,
      color: C.slate200,
    },
    { label: idCopy.operator, value: report.identity.org, color: C.slate200 },
    { label: idCopy.asn, value: asnDisplay, color: C.slate200, mono: true },
    {
      label: idCopy.fraudScore,
      value: `${report.identity.fraudScore}/100${lp}${fraudTag}${rp}`,
      color:
        report.identity.fraudScore >= 50
          ? C.leak
          : report.identity.fraudScore >= 25
            ? C.warn
            : C.sig,
    },
    {
      label: idCopy.blacklist,
      value: report.identity.blacklist.length
        ? idCopy.blacklistHit(report.identity.blacklist.length)
        : idCopy.blacklistClean,
      color: report.identity.blacklist.length ? C.leak : C.sig,
    },
    {
      label: idCopy.humanTrafficShort,
      value:
        humanPct != null
          ? `${fmtPct(humanPct)}%${lp}${humanTag}${rp}`
          : idCopy.humanNoData,
      color:
        humanPct == null
          ? C.slate500
          : humanPct >= 70
            ? C.sig
            : humanPct >= 40
              ? C.warn
              : C.leak,
    },
  ];
  const colsX = PAD + (locale === "en" ? 452 : 430);
  const colsTotal = W - PAD - 28 - colsX;
  const weights = locale === "en" ? [1.45, 1.12, 0.7, 1.0, 0.6, 1.05] : [1.35, 1.15, 0.72, 0.92, 0.62, 1.16];
  const weightSum = weights.reduce((a, b) => a + b, 0);
  let cx = colsX;
  cols.forEach((c, i) => {
    const colW = (colsTotal * weights[i]) / weightSum;
    text(ctx, c.label, cx, idCy - 16, { font: font(12), color: C.slate500 });
    const valFont = font(14, 500, c.mono ? MONO : SANS);
    ctx.font = valFont;
    text(ctx, truncate(ctx, c.value, colW - 14), cx, idCy + 12, {
      font: valFont,
      color: c.color,
    });
    cx += colW;
  });

  if (rtt) {
    ctx.strokeStyle = C.line;
    ctx.beginPath();
    ctx.moveTo(PAD + 28, idY + 92);
    ctx.lineTo(W - PAD - 28, idY + 92);
    ctx.stroke();

    const ly = idY + 122;
    let rx = PAD + 28;
    ctx.font = font(12, 400, MONO);
    text(ctx, idCopy.rttTitle, rx, ly, {
      font: font(12, 400, MONO),
      color: C.slate500,
    });
    rx += ctx.measureText(idCopy.rttTitle).width + 30;

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
    if (rtt.clientMinMS != null) item(idCopy.rttLatency, rtt.clientMinMS);
    if (rtt.serverTCPMS != null) item(idCopy.rttTcp, rtt.serverTCPMS);
    if (rtt.deltaMS != null) item(idCopy.rttDelta, rtt.deltaMS);

    text(ctx, idCopy.rttBeta, W - PAD - 28, ly, {
      font: font(11),
      color: C.slate600,
      align: "right",
    });
  }

  const detY = idY + idH + 20;
  const detH = H - detY - PAD - 34;
  const detW = (W - PAD * 2 - 40) / 3;
  const detStart = 68;
  const detStep = 33;
  const sections = [
    {
      title: m.home.groups.consistency,
      rows: report.consistency,
      state: report.groups[1]?.state ?? "unknown",
    },
    {
      title: m.home.groups.leak,
      rows: report.leaks,
      state: report.groups[2]?.state ?? "unknown",
    },
    {
      title: m.home.groups.environment,
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
    if (i === 0 && report.scenario === "daily") {
      text(ctx, m.home.readout.referenceOnly, sx + detW - 24, detY + 32, {
        font: font(11, 400, MONO),
        color: C.slate500,
        align: "right",
      });
    } else {
      dot(
        ctx,
        sx + detW - 26,
        detY + 32,
        STATE_COLOR[s.state],
        STATE_SOFT[s.state],
      );
    }
    s.rows.slice(0, 5).forEach((r, j) => {
      const ry = detY + detStart + j * detStep;
      dot(ctx, sx + 28, ry, STATE_COLOR[r.state], STATE_SOFT[r.state]);
      ctx.font = font(13);
      text(ctx, r.label, sx + 44, ry, { font: font(13), color: C.slate400 });
      const labelW = ctx.measureText(r.label).width;
      const valMaxW = Math.max(96, detW - 44 - labelW - 36);
      text(ctx, truncate(ctx, r.value, valMaxW), sx + detW - 24, ry, {
        font: font(13),
        color: C.slate200,
        align: "right",
      });
    });
  });

  dot(ctx, PAD + 4, H - PAD + 18, C.radar, C.sigSoft);
  text(ctx, locale === "en" ? "detectradar.com/en/" : "detectradar.com", PAD + 18, H - PAD + 18, {
    font: font(15, 600, MONO),
    color: C.slate200,
  });
  text(ctx, sc.footerTagline, W - PAD, H - PAD + 18, {
    font: font(12.5),
    color: C.slate500,
    align: "right",
  });

  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error(m.common.imageEmptyError))),
      "image/png",
    );
  });
}
