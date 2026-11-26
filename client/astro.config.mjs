// @ts-check
import { defineConfig } from 'astro/config';
import { access, copyFile } from 'node:fs/promises';

import svelte from '@astrojs/svelte';
import sitemap from '@astrojs/sitemap';

import tailwindcss from '@tailwindcss/vite';

/** @type {Record<string, { priority: number; changefreq: 'weekly' | 'yearly'; lastmod: string }>} */
const SITEMAP_META = {
  '/': { priority: 1.0, changefreq: 'weekly', lastmod: '2026-11-25' },
  '/dns-leak-test/': { priority: 0.9, changefreq: 'weekly', lastmod: '2026-11-25' },
  '/webrtc-leak-test/': { priority: 0.9, changefreq: 'weekly', lastmod: '2026-11-24' },
  '/ipv6-leak-test/': { priority: 0.9, changefreq: 'weekly', lastmod: '2026-11-24' },
  '/macos/': { priority: 0.7, changefreq: 'weekly', lastmod: '2026-11-26' },
  '/about/': { priority: 0.8, changefreq: 'weekly', lastmod: '2026-11-25' },
  '/contact/': { priority: 0.3, changefreq: 'yearly', lastmod: '2026-11-21' },
  '/privacy/': { priority: 0.3, changefreq: 'yearly', lastmod: '2026-11-21' },
  '/terms/': { priority: 0.3, changefreq: 'yearly', lastmod: '2026-11-21' },
};

// https://astro.build/config
export default defineConfig({
  // 自托管构建可用 PUBLIC_SITE_URL 覆盖（影响 canonical/sitemap），默认官方站
  site: process.env.PUBLIC_SITE_URL ?? 'https://detectradar.com',
  trailingSlash: 'always',

  integrations: [
    svelte(),
    sitemap({
      changefreq: 'weekly',
      priority: 0.7,
      serialize(item) {
        const pathname = new URL(item.url).pathname;
        const meta = SITEMAP_META[pathname];
        if (!meta) return item;
        return {
          ...item,
          priority: meta.priority,
          changefreq: /** @type {NonNullable<typeof item.changefreq>} */ (/** @type {unknown} */ (meta.changefreq)),
          lastmod: meta.lastmod,
        };
      },
    }),
    {
      name: 'sitemap-alias',
      hooks: {
        'astro:build:done': async ({ dir }) => {
          // 仅单分片时将 /sitemap.xml 直接平铺为 <urlset>（sitemap-0.xml），
          // 避免 ClaudeBot/GPTBot 等 AI 爬虫停在两层 <sitemapindex> 不再下钻；
          // 同时保留 sitemap-index.xml 与 sitemap-0.xml 兼容已登记的搜索引擎。
          let hasMultipleShards = false;
          try {
            await access(new URL('sitemap-1.xml', dir));
            hasMultipleShards = true;
          } catch {
            hasMultipleShards = false;
          }
          const source = hasMultipleShards ? 'sitemap-index.xml' : 'sitemap-0.xml';
          await copyFile(new URL(source, dir), new URL('sitemap.xml', dir));
        },
      },
    },
  ],

  vite: {
    plugins: [tailwindcss()]
  }
});
