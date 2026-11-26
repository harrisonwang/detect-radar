const AUTOMATED_CRAWLER_UA_RE =
  /\b(?:Googlebot|Google-InspectionTool|Storebot-Google|GoogleOther|AdsBot-Google|APIs-Google|Mediapartners-Google|bingbot|BingPreview|MicrosoftPreview|adidxbot|msnbot|Baiduspider|YisouSpider|Sogou|360Spider|HaosouSpider|Bytespider|YandexBot|YandexRenderResourcesBot|YandexMobileBot|Applebot|DuckDuckBot|PetalBot|SeznamBot|NaverBot|Yeti|GPTBot|OAI-SearchBot|ChatGPT-User|ClaudeBot|Claude-SearchBot|Claude-User|PerplexityBot|Perplexity-User|Meta-ExternalAgent|Meta-ExternalFetcher|Meta-WebIndexer|AhrefsBot|SemrushBot|MJ12bot|DotBot|facebookexternalhit|Twitterbot|TelegramBot|Slackbot|Discordbot)\b/i;

export function isAutomatedCrawler(userAgent?: string): boolean {
  const ua =
    userAgent ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '');
  return AUTOMATED_CRAWLER_UA_RE.test(ua);
}
