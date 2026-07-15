// @ts-check
import { defineConfig } from 'astro/config';

import svelte from '@astrojs/svelte';
import sitemap from '@astrojs/sitemap';

import tailwindcss from '@tailwindcss/vite';

// https://astro.build/config
export default defineConfig({
  site: 'https://detectradar.com',
  trailingSlash: 'never',

  integrations: [svelte(), sitemap({ changefreq: 'weekly', priority: 0.7 })],

  vite: {
    plugins: [tailwindcss()]
  }
});