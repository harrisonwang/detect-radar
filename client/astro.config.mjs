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

export default defineConfig({
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
