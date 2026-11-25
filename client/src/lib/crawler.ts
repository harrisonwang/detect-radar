/**
 * 识别搜索引擎、AI 索引及社媒链接预览爬虫。
 * 仅用于跳过页面挂载时的自动检测请求，避免消耗抓取预算并向后端写入假扫描/DNS 探针；
 * 不拦截用户手动点击「开始检测 / 重新检测」，也不依赖 navigator.webdriver
 * （真实用户常使用 Playwright / Selenium / 反检测浏览器通过本站排查自动化特征）。
 */
const AUTOMATED_CRAWLER_UA_RE =
  /\b(?:Googlebot|Google-InspectionTool|Storebot-Google|GoogleOther|AdsBot-Google|APIs-Google|Mediapartners-Google|bingbot|BingPreview|MicrosoftPreview|adidxbot|msnbot|Baiduspider|YisouSpider|Sogou|360Spider|HaosouSpider|Bytespider|YandexBot|YandexRenderResourcesBot|YandexMobileBot|Applebot|DuckDuckBot|PetalBot|SeznamBot|NaverBot|Yeti|GPTBot|OAI-SearchBot|ChatGPT-User|ClaudeBot|Claude-SearchBot|Claude-User|PerplexityBot|Perplexity-User|Meta-ExternalAgent|Meta-ExternalFetcher|Meta-WebIndexer|AhrefsBot|SemrushBot|MJ12bot|DotBot|facebookexternalhit|Twitterbot|TelegramBot|Slackbot|Discordbot)\b/i;

export function isAutomatedCrawler(userAgent?: string): boolean {
  const ua =
    userAgent ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '');
  return AUTOMATED_CRAWLER_UA_RE.test(ua);
}
