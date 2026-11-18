// @ts-check
import { defineConfig } from 'astro/config';
import { copyFile } from 'node:fs/promises';

import svelte from '@astrojs/svelte';
import sitemap from '@astrojs/sitemap';

import tailwindcss from '@tailwindcss/vite';

// https://astro.build/config
export default defineConfig({
  // 自托管构建可用 PUBLIC_SITE_URL 覆盖（影响 canonical/sitemap），默认官方站
  site: process.env.PUBLIC_SITE_URL ?? 'https://detectradar.com',
  trailingSlash: 'always',

  integrations: [
    svelte(),
    sitemap({ changefreq: 'weekly', priority: 0.7 }),
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