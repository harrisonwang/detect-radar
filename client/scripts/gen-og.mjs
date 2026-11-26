#!/usr/bin/env node
import { execFile } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);
const clientDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const publicDir = path.join(clientDir, 'public');
const CHROME =
  process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const BRAND_SVG = `<svg viewBox="0 0 576 512" aria-hidden="true"><path fill="#4ADE80" d="M288-16c17.7 0 32 14.3 32 32l0 18.3c98.1 14 175.7 91.6 189.7 189.7l18.3 0c17.7 0 32 14.3 32 32s-14.3 32-32 32l-18.3 0c-14 98.1-91.6 175.7-189.7 189.7l0 18.3c0 17.7-14.3 32-32 32s-32-14.3-32-32l0-18.3C157.9 463.7 80.3 386.1 66.3 288L48 288c-17.7 0-32-14.3-32-32s14.3-32 32-32l18.3 0C80.3 125.9 157.9 48.3 256 34.3L256 16c0-17.7 14.3-32 32-32zM131.2 288c12.7 62.7 62.1 112.1 124.8 124.8l0-12.8c0-17.7 14.3-32 32-32s32 14.3 32 32l0 12.8c62.7-12.7 112.1-62.1 124.8-124.8L432 288c-17.7 0-32-14.3-32-32s14.3-32 32-32l12.8 0C432.1 161.3 382.7 111.9 320 99.2l0 12.8c0 17.7-14.3 32-32 32s-32-14.3-32-32l0-12.8C193.3 111.9 143.9 161.3 131.2 224l12.8 0c17.7 0 32 14.3 32 32s-14.3 32-32 32l-12.8 0zM288 208a48 48 0 1 1 0 96 48 48 0 1 1 0-96z"/></svg>`;

function renderRadarGraphic() {
  return `
    <div class="right">
      <svg viewBox="0 0 460 408" width="420" height="372">
        <g fill="none" stroke="rgba(148,163,184,0.16)" stroke-width="1">
          <circle cx="230" cy="210" r="136" />
          <circle cx="230" cy="210" r="102" />
          <circle cx="230" cy="210" r="68" />
          <circle cx="230" cy="210" r="34" />
          <line x1="230" y1="74" x2="230" y2="346" />
          <line x1="94" y1="210" x2="366" y2="210" />
        </g>
        <polygon points="230,84.8 314.3,210 230,329.7 100.8,210" fill="rgba(251,191,36,0.13)" stroke="#fbbf24" stroke-width="2.5" stroke-linejoin="round" />
        <g stroke="#020617" stroke-width="1.5">
          <circle cx="230" cy="84.8" r="5" fill="#4ade80" />
          <circle cx="314.3" cy="210" r="5.5" fill="#fbbf24" />
          <circle cx="230" cy="329.7" r="5" fill="#4ade80" />
          <circle cx="100.8" cy="210" r="5" fill="#4ade80" />
        </g>
        <g text-anchor="middle" font-family="Inter, -apple-system, BlinkMacSystemFont, sans-serif">
          <text x="230" y="44" font-size="15" fill="#94a3b8">Identity</text>
          <text x="230" y="64" font-size="13" fill="#4ade80" font-family="ui-monospace, Menlo, monospace">92</text>
          <text x="412" y="204" font-size="15" fill="#94a3b8">Consistency</text>
          <text x="412" y="224" font-size="13" fill="#fbbf24" font-family="ui-monospace, Menlo, monospace">62</text>
          <text x="230" y="372" font-size="15" fill="#94a3b8">Leaks</text>
          <text x="230" y="392" font-size="13" fill="#4ade80" font-family="ui-monospace, Menlo, monospace">88</text>
          <text x="46" y="204" font-size="15" fill="#94a3b8">Fingerprint</text>
          <text x="46" y="224" font-size="13" fill="#4ade80" font-family="ui-monospace, Menlo, monospace">95</text>
        </g>
      </svg>
      <div class="risk">Overall Risk <b>Medium</b></div>
    </div>`;
}

function renderPanelGraphic({ label, status, statusColor, rows }) {
  const rowsHtml = rows
    .map(
      (r) => `
      <div class="p-row">
        <span class="p-key">${r.k}</span>
        <span class="p-val" style="color:${r.color || '#e2e8f0'}">${r.v}</span>
      </div>`,
    )
    .join('');
  return `
    <div class="right panel-wrap">
      <div class="panel">
        <div class="p-head">
          <span class="p-label">${label}</span>
          <span class="p-pill" style="color:${statusColor};background:${statusColor}22;border-color:${statusColor}55">${status}</span>
        </div>
        <div class="p-rows">${rowsHtml}</div>
      </div>
    </div>`;
}

const PAGES = [
  {
    file: 'og-default-en.png',
    h1: 'Network Environment<br/><em>Radar</em>',
    sub: 'Check exit IP purity, timezone &amp; language match,<br/>DNS / WebRTC / IPv6 leaks, and browser fingerprint.',
    badge: { tone: '#fbbf24', tag: 'MISMATCH', title: 'Timezone mismatches exit IP', fact: 'Browser UTC+8 ↔ Exit UTC-5' },
    rightHtml: renderRadarGraphic(),
    domain: 'detectradar.com/en/',
  },
  {
    file: 'og-dns-en.png',
    h1: '<em>DNS</em> Leak Test',
    sub: 'Authoritative NS + DNSTap captures your real recursive<br/>DNS resolvers and detects ISP bypass leaks.',
    badge: { tone: '#4ade80', tag: 'NO LEAK', title: 'Public DNS resolver verified', fact: 'Cloudflare 1.1.1.1 · 3 probe rounds' },
    rightHtml: renderPanelGraphic({
      label: 'AUTHORITATIVE NS TRACE',
      status: 'NO LEAK',
      statusColor: '#4ade80',
      rows: [
        { k: 'HTTPS Exit', v: 'US · Residential ISP' },
        { k: 'Resolver', v: 'Cloudflare Public DNS', color: '#4ade80' },
        { k: 'Policy', v: 'Cross-Region + Proxy Bypass' },
        { k: 'Compare', v: 'Last 2 runs saved locally' },
      ],
    }),
    domain: 'detectradar.com/en/dns-leak-test/',
  },
  {
    file: 'og-webrtc-en.png',
    h1: '<em>WebRTC</em> Leak Test',
    sub: 'Inspect exposed public ICE candidates, LAN / CGNAT IPs,<br/>and browser mDNS (*.local) privacy protection.',
    badge: { tone: '#4ade80', tag: 'PROTECTED', title: 'Local IP hidden via mDNS', fact: 'Public ICE matches exit IP · Zero retention' },
    rightHtml: renderPanelGraphic({
      label: 'STUN ICE INSPECTION',
      status: 'VERIFIED',
      statusColor: '#4ade80',
      rows: [
        { k: 'Public ICE', v: 'Matches HTTPS exit', color: '#4ade80' },
        { k: 'Local IP', v: 'Hidden via *.local mDNS' },
        { k: 'IPv6 ICE', v: 'Same /64 privacy prefix' },
        { k: 'Storage', v: 'Local attribution · No log' },
      ],
    }),
    domain: 'detectradar.com/en/webrtc-leak-test/',
  },
  {
    file: 'og-ipv6-en.png',
    h1: '<em>IPv6</em> Leak Test',
    sub: 'Concurrent IPv4-only and IPv6-only probes catch ISP<br/>IPv6 bypass leaks even with WebRTC disabled.',
    badge: { tone: '#f87171', tag: 'LEAK', title: 'IPv6 bypassing proxy via local ISP', fact: 'IPv4 US Hosting ↔ IPv6 Local Broadband' },
    rightHtml: renderPanelGraphic({
      label: 'DUAL-STACK PROBE',
      status: 'ISP BYPASS',
      statusColor: '#f87171',
      rows: [
        { k: '4.detectradar', v: 'US · Datacenter Exit' },
        { k: '6.detectradar', v: 'Local ISP Direct', color: '#f87171' },
        { k: 'WebRTC Dep', v: 'None (independent probe)' },
        { k: 'Compare', v: 'Before/after diff built in' },
      ],
    }),
    domain: 'detectradar.com/en/ipv6-leak-test/',
  },
  {
    file: 'og-about-en.png',
    h1: 'How <em>DetectRadar</em><br/>Works',
    sub: 'Open-source leak detection methodology, data sources,<br/>CLI curl/dig checks, and privacy boundaries.',
    badge: { tone: '#4ade80', tag: 'MIT OPEN', title: 'Observable facts, zero signup', fact: 'curl detectradar.com/en/ · SKILL.md ready' },
    rightHtml: renderPanelGraphic({
      label: 'METHODOLOGY & CLI',
      status: 'OPEN SOURCE',
      statusColor: '#4ade80',
      rows: [
        { k: 'IP Intel', v: 'Local L1+L2 · Optional L3' },
        { k: 'DNS / IPv6', v: 'Global Access-ISP Policy v3' },
        { k: 'CLI', v: 'curl detectradar.com/en/' },
        { k: 'AI Agents', v: '/en/SKILL.md & /llms.txt' },
      ],
    }),
    domain: 'detectradar.com/en/about/',
  },
];

function buildHtml(page) {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<style>
  :root { --space-950: #020617; --sig: #4ade80; --warn: #fbbf24; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html, body { width: 1200px; height: 630px; }
  body {
    position: relative;
    overflow: hidden;
    background: var(--space-950);
    color: #fff;
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', sans-serif;
  }
  .grid {
    position: absolute; inset: 0;
    background-image:
      linear-gradient(rgba(34, 197, 94, 0.05) 1px, transparent 1px),
      linear-gradient(90deg, rgba(34, 197, 94, 0.05) 1px, transparent 1px);
    background-size: 40px 40px;
  }
  .glow {
    position: absolute; top: -260px; right: -280px; width: 900px; height: 900px;
    background: radial-gradient(circle, rgba(34, 197, 94, 0.13) 0%, transparent 62%);
  }
  .glow2 {
    position: absolute; bottom: -320px; left: -300px; width: 820px; height: 820px;
    background: radial-gradient(circle, rgba(37, 99, 235, 0.1) 0%, transparent 62%);
  }
  .wrap {
    position: relative;
    display: flex; align-items: center; gap: 40px;
    height: 100%; padding: 0 68px 36px;
  }
  .left { flex: 1; min-width: 0; }
  .brand { display: flex; align-items: center; gap: 14px; margin-bottom: 24px; }
  .brand svg { width: 32px; height: 32px; }
  .brand span { font-size: 24px; font-weight: 600; letter-spacing: 0.02em; font-family: ui-monospace, Menlo, monospace; }
  .brand em { font-style: normal; color: var(--sig); }
  h1 { font-size: 54px; font-weight: 700; line-height: 1.12; letter-spacing: -0.025em; }
  h1 em { font-style: normal; color: var(--sig); }
  .sub { margin-top: 20px; font-size: 21px; line-height: 1.5; color: #94a3b8; }
  .finding {
    display: inline-flex; align-items: center; gap: 14px;
    margin-top: 28px; padding: 14px 18px;
    border: 1px solid rgba(255, 255, 255, 0.09); border-radius: 12px;
    background: rgba(30, 41, 59, 0.58);
  }
  .sev {
    flex: none; padding: 6px 10px; border-radius: 6px;
    font-family: ui-monospace, Menlo, monospace; font-size: 12px; font-weight: 600;
    letter-spacing: 0.04em;
  }
  .ftitle { font-size: 16px; color: #e2e8f0; font-weight: 500; }
  .ffact { margin-top: 4px; font-size: 13.5px; color: #94a3b8; font-family: ui-monospace, Menlo, monospace; }
  .right { flex: none; width: 420px; display: flex; flex-direction: column; align-items: center; }
  .risk { margin-top: 4px; display: flex; align-items: baseline; gap: 10px; font-size: 15px; color: #64748b; }
  .risk b { font-size: 18px; font-weight: 600; color: var(--warn); }
  .panel {
    width: 100%; border-radius: 16px;
    border: 1px solid rgba(148, 163, 184, 0.16);
    background: rgba(15, 23, 42, 0.85);
    padding: 24px 26px;
    box-shadow: 0 24px 60px -16px rgba(2, 6, 23, 0.85);
  }
  .p-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding-bottom: 16px; border-bottom: 1px solid rgba(148, 163, 184, 0.12); }
  .p-label { font-family: ui-monospace, Menlo, monospace; font-size: 12px; color: #94a3b8; letter-spacing: 0.08em; }
  .p-pill { font-family: ui-monospace, Menlo, monospace; font-size: 12px; font-weight: 600; padding: 4px 10px; border-radius: 999px; border: 1px solid; }
  .p-rows { display: flex; flex-direction: column; gap: 14px; margin-top: 18px; }
  .p-row { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; font-size: 15px; }
  .p-key { color: #64748b; font-size: 13.5px; font-family: ui-monospace, Menlo, monospace; flex: none; }
  .p-val { font-weight: 500; text-align: right; }
  .foot {
    position: absolute; bottom: 34px; left: 68px;
    display: flex; align-items: center; gap: 14px;
    font-size: 17px; color: #64748b;
  }
  .foot b { color: #e2e8f0; font-weight: 600; font-family: ui-monospace, Menlo, monospace; }
  .sep { width: 3px; height: 3px; border-radius: 50%; background: #334155; }
</style>
</head>
<body>
  <div class="grid"></div>
  <div class="glow"></div>
  <div class="glow2"></div>
  <div class="wrap">
    <div class="left">
      <div class="brand">${BRAND_SVG}<span>Detect<em>Radar</em></span></div>
      <h1>${page.h1}</h1>
      <p class="sub">${page.sub}</p>
      <div class="finding">
        <span class="sev" style="color:${page.badge.tone};background:${page.badge.tone}22">${page.badge.tag}</span>
        <div>
          <div class="ftitle">${page.badge.title}</div>
          <div class="ffact">${page.badge.fact}</div>
        </div>
      </div>
    </div>
    ${page.rightHtml}
  </div>
  <div class="foot">
    <b>${page.domain}</b>
    <i class="sep"></i>Free<i class="sep"></i>No signup<i class="sep"></i>Open source
  </div>
</body>
</html>`;
}

async function main() {
  const tmpDir = await mkdtemp(path.join(os.tmpdir(), 'detectradar-og-'));
  try {
    for (const page of PAGES) {
      const htmlPath = path.join(tmpDir, `${page.file}.html`);
      const outPath = path.join(publicDir, page.file);
      await writeFile(htmlPath, buildHtml(page), 'utf8');
      await execFileAsync(CHROME, [
        '--headless=new',
        '--disable-gpu',
        '--hide-scrollbars',
        '--force-device-scale-factor=1',
        '--window-size=1200,630',
        `--screenshot=${outPath}`,
        `file://${htmlPath}`,
      ]);
      console.log(`Generated ${path.relative(clientDir, outPath)}`);
    }
  } finally {
    await rm(tmpDir, { recursive: true, force: true });
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
