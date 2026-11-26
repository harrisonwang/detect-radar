import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

const docs = defineCollection({
  loader: glob({ pattern: '**/*.md', base: './src/content/docs' }),
  schema: z.object({
    title: z.string(),
    h1: z.string(),
    description: z.string(),
    canonical: z.string(),
    locale: z.enum(['zh-CN', 'en']),
    ogImage: z.string().optional(),
    pageType: z.enum(['article', 'static']).default('static'),
    datePublished: z.string().optional(),
    dateModified: z.string(),
    hasFaqs: z.boolean().optional(),
  }),
});

export const collections = { docs };
