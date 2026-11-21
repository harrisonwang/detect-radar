// @ts-check
import { defineConfig } from 'astro/config';
import { copyFile } from 'node:fs/promises';

import svelte from '@astrojs/svelte';
import sitemap from '@astrojs/sitemap';

import tailwindcss from '@tailwindcss/vite';

/** @type {Record<string, { priority: number; changefreq: 'weekly' | 'yearly'; lastmod: string }>} */
const SITEMAP_META = {
  '/': { priority: 1.0, changefreq: 'weekly', lastmod: '2026-11-21' },
  '/dns-leak-test/': { priority: 0.9, changefreq: 'weekly', lastmod: '2026-11-21' },
  '/about/': { priority: 0.8, changefreq: 'weekly', lastmod: '2026-11-21' },
  '/contact/': { priority: 0.3, changefreq: 'yearly', lastmod: '2026-11-21' },
  '/privacy/': { priority: 0.3, changefreq: 'yearly', lastmod: '2026-11-19' },
  '/terms/': { priority: 0.3, changefreq: 'yearly', lastmod: '2026-10-23' },
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
          await copyFile(
            new URL('sitemap-index.xml', dir),
            new URL('sitemap.xml', dir),
          );
        },
      },
    },
  ],

  vite: {
    plugins: [tailwindcss()]
  }
});
