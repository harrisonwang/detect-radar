#!/usr/bin/env node
/**
 * 将已构建的规范 URL 推送到 IndexNow（Bing / Yandex / Naver / Seznam 等共享）。
 *
 * 用法：
 *   npm run indexnow                       # 从 dist/sitemap.xml 读取全部 URL 并推送
 *   npm run indexnow -- --dry-run          # 仅打印请求体，不发起网络请求
 *   npm run indexnow -- https://detectradar.com/about/  # 只推指定 URL
 *
 * 约定：
 *   - 密钥文件放在 public/<32位十六进制>.txt，内容与文件名一致（也可通过 INDEXNOW_KEY 指定）
 *   - 请务必在静态产物同步到线上之后再执行，确保 Bingbot 拉到的是最新版本
 */
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ENDPOINT = process.env.INDEXNOW_ENDPOINT ?? 'https://api.indexnow.org/indexnow';
const clientDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const publicDir = path.join(clientDir, 'public');
const sitemapPath = path.join(clientDir, 'dist', 'sitemap.xml');

const args = process.argv.slice(2);
const dryRun = args.includes('--dry-run');
const explicitUrls = args.filter((a) => !a.startsWith('--'));

async function resolveKey() {
  if (process.env.INDEXNOW_KEY) {
    return process.env.INDEXNOW_KEY.trim();
  }
  const files = await readdir(publicDir);
  const keyFile = files.find((f) => /^[a-f0-9]{32}\.txt$/i.test(f));
  if (!keyFile) {
    throw new Error(`未在 ${publicDir} 找到 IndexNow 密钥文件（<32位十六进制>.txt），可通过 INDEXNOW_KEY 指定`);
  }
  const expected = keyFile.slice(0, -4);
  const content = (await readFile(path.join(publicDir, keyFile), 'utf8')).trim();
  if (content !== expected) {
    throw new Error(`密钥文件 ${keyFile} 内容与文件名不一致`);
  }
  return expected;
}

async function resolveUrls() {
  if (explicitUrls.length > 0) {
    return explicitUrls;
  }
  let xml;
  try {
    xml = await readFile(sitemapPath, 'utf8');
  } catch {
    throw new Error(`未找到 ${sitemapPath}，请先执行 npm run build`);
  }
  const urls = Array.from(xml.matchAll(/<loc>([^<]+)<\/loc>/g), (m) => m[1].trim());
  if (urls.length === 0) {
    throw new Error(`${sitemapPath} 中未解析到任何 <loc> URL`);
  }
  return urls;
}

async function main() {
  const key = await resolveKey();
  const urlList = await resolveUrls();
  const parsed = urlList.map((u) => new URL(u));
  const host = parsed[0].host;
  const origin = parsed[0].origin;
  for (const u of parsed) {
    if (u.host !== host) {
      throw new Error(`urlList 包含跨域名地址：${u.href}（预期 host=${host}）`);
    }
  }

  const payload = {
    host,
    key,
    keyLocation: `${origin}/${key}.txt`,
    urlList,
  };

  if (dryRun) {
    console.log(`[dry-run] POST ${ENDPOINT}`);
    console.log(JSON.stringify(payload, null, 2));
    return;
  }

  const res = await fetch(ENDPOINT, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json; charset=utf-8' },
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(15000),
  });
  const body = await res.text();
  if (res.status === 200 || res.status === 202) {
    console.log(`IndexNow 推送成功：HTTP ${res.status}，共提交 ${urlList.length} 个 URL（host=${host}）`);
    return;
  }
  throw new Error(`IndexNow 推送失败：HTTP ${res.status}${body ? ` — ${body}` : ''}`);
}

main().catch((err) => {
  console.error(err instanceof Error ? err.message : String(err));
  process.exit(1);
});
