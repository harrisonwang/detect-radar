# DetectRadar Agent 工程准则与协作方法论 (`AGENTS.md`)

> 本文件供协助开发 `detect-radar` 的 AI Agent 阅读并严格遵守。前半部分定义工作方法论与品味红线，后半部分定义代码事实边界与前后端工程约束。

---

## 1. 工作方法论与品味红线

1. **事实优先、极度克制（反营销腔与 SEO 水文）**
   - 任何文案、FAQ、API 文档、诊断说明必须以仓库真实代码实现与接口实测返回为唯一依据，严禁脑补字段或夸大能力；能力边界与局限（如漏报条件、不扣分项）必须如实写明。
   - 工具本体永远是视觉主角，说明文档只是服务排障的配角（低对比度、克制强调色、无营销弹窗/引流条/无意义的置灰按钮）。
   - 调研与方案必须拆清维度（如 SEO 与 GEO 分开分析），基于真实竞品与现网数据提出可分批落地的动作，拒绝泛泛而谈。
   - 诊断卡内的交互可视化只描绘客观存在或实测观测到的实体与链路，不脑补服务端无法感知的中间黑盒（如假想路由器/代理层），不在真实结果卡内混入模拟对比态以免与实测结论同屏矛盾，也不照搬教学播放器式控件（重放/单步）与「重新检测」产生心智冲突；原理演示统一归口 `/about/`。

2. **评审顺序：先骨后皮（UI/UX 排最后）**
   - 页面改版或新增核心页时，默认按 **Technical SEO（抓取与语义）→ GEO / AI Agent（机器可读与可引用）→ 目标网络工程师（排障任务闭环）→ UI/UX（视觉层级与细节收口）** 顺序评审与调整，最后再打磨视觉。

3. **眼见为实与机器视角可视化**
   - 交付前必须完成端到端实测：类型检查、静态构建、真实接口/HTTP 状态检查，以及桌面端与 375px 移动端渲染核验。
   - 涉及 SEO/GEO、结构化数据、配置生效等肉眼不可见的改动，优先提供直观对比或透视方式供验收，不口头宣称“应该生效”。

4. **可控优先、隔离试错、严禁擅自提交**
   - 未经明确要求，**绝不自动执行 `git commit` 或 `git push`**，不擅自修改线上生产配置、DNS 或历史数据。
   - 遇到不可逆或高影响分岔（如绑定域名、部署形态、是否迁移旧库），先给出精炼选项确认再执行。
   - 基础设施改动偏好透明可控、易排障的直白方案，不无故引入多余黑盒抽象。

5. **“一文两用”文档习惯**
   - 指南、运维与排障类文档遵循动静分层：**上半屏说人话**（1–2 分钟扫懂核心组成、大致流程与主要坑位）；**下半屏放精确硬指令**（零歧义变量、可直接复制执行的完整命令与验收标准）。

---

## 2. 代码与检测逻辑事实边界（写文档与改 UI 必须对齐）

### 2.1 DNS 泄露判定（Policy v2：`server/dnsassessment/assessment.go`、`client/src/lib/dnsPresentation.ts`）
- **唯一触发 `泄露`（`mainland-carrier`）的条件**：出口 IP 位于海外（`exit.Country != "CN"`）**且**权威 NS/DNSTap 捕获到境内运营商接入网解析器（`country == "CN"` 且 `network_type == "access"`）。
- **明确不判泄露**：
  - `exit.Country == "CN"` 直接返回 `clear`（`domestic-exit`，页面显示 `未泄露`）。
  - 已知公共解析器（AliDNS `223.5.5.5`/`223.6.6.6`、DNSPod `119.29.29.29`、AdGuard、Cloudflare、Google、Quad9、OpenDNS、NextDNS）归为 `public` 并跳过判定；通用机房 `hosting` 解析器同样跳过。
- **页面状态词固定为**：`泄露`、`未泄露`、`无法判断`、`无 DNS 结果`。
- **缓存边界**：前端每次扫描生成全新随机探针子域 `r{rand}-{testID}.<zone>`，不受历史 A/AAAA DNS 缓存影响；复测不变通常源自未重置的 DoH/QUIC 长连接或未重启代理服务。
- **并发采样与跨国公共 DNS 提示**：专项页 `measureDNS`（`client/src/lib/dnsCheck.ts`）并发发起 `GET /api/v1/ip` 与错峰 3 轮 `sampleDNSRound()`，经 `DNSProgress`（`probing` → `collecting` → `assessing`）实时同步路径动画；仅在 `未泄露` 且全部为公共 DNS 且解析器归属国与出口国不同时，在结论区提示「公共 DNS 和出口 IP 的归属地可以不同。」以防误判。

### 2.2 一致性、泄露与指纹边界（`client/src/lib/report.ts`、`client/src/lib/collectors/`）
- **一致性维度**：仅比对浏览器 `Intl` 时区（按 UTC 偏移归一）与出口 IP 时区、以及 `navigator.languages` 与出口国家常用语；**硬件参数行仅作展示，不计入冲突扣分**。
- **WebGL 与指纹隐私**：WebGL 仅比对 `unmasked_renderer` 与 `navigator.platform`（操作系统一致性，**不与出口国家交叉比对**）；Canvas / Audio / WebGL 绘制结果与扩展列表在本地计算 SHA-256 哈希上传，仅 `unmasked_vendor` 与 `unmasked_renderer` 原文上传。
- **WebRTC 专项页判定（`client/src/lib/webrtcCheck.ts`）**：页面状态词固定为 `泄露`、`未泄露`、`无法判断`（严禁自造“存在差异”等状态词）；测到与出口 IP 不同的公网 IP（排除同连接正常 IPv4/IPv6 双栈）判为 `泄露`；与出口一致、正常双栈、仅返回 mDNS（`*.local`）或本机内网地址（无额外公网 IP）、已禁用 WebRTC 均判为 `未泄露`；缺少出口 IP 时判为 `无法判断`。
- **IPv6 探针边界**：浏览器端 `ipv6.ts` 复用 WebRTC ICE 候选——若浏览器禁用 WebRTC，页面侧 IPv6 探测将同步跳过（可能漏报）；文档与排障说明须提示配合 `curl -6 -s https://detectradar.com/api/v1/ip` 复核双栈。
- **诊断总结词**：无异常时统一表述为 `未见明显异常`（避免与代码脱节的自造词）。

### 2.3 对外 JSON API 契约（`server/cmd/server/main.go`、`server/internal/handler/`）
- `GET /api/v1/ip`：返回当前连接出口信息；`usage_type` 枚举固定为 `"isp" | "hosting" | "unknown"`（不存在 `"datacenter"`）；默认仅走本地 `L1+L2`（`is_vpn`/`is_proxy` 默认 `false`），附加 `?deep=true` 才触发上游 `L3`。
- `GET /api/v1/ip/reputation`：返回 DNSBL/PBL（`blacklist_hit`、`blacklist_sources`、`pbl_listed`）、开放代理端口（`open_ports`、`open_proxy_port`、`weak_proxy_port`）、`is_tor_exit`、`exposure_tags`、`recommendation`。
- 终端免鉴权 3 步 DNS 探测：`POST /api/v1/leaks/dns`（返回 `id` 与 `test_domain`）→ `dig +short "r1.<test_domain>"` → `GET /api/v1/leaks/dns/<id>`（返回 `dns_servers`、`assessment`）。

---

## 3. 前端架构、SEO/GEO 与 UI 约束（`client/`）

- **技术栈**：Astro 5（`trailingSlash: 'always'`，SSG）+ Svelte 5 + Tailwind CSS v4；后端为 Go + Fiber（`CGO_ENABLED=0` 纯 Go 构建）。
- **页面分工与 `<h*>` 大纲**：
  - 核心检测工具页（`/`、`/dns-leak-test/`、`/webrtc-leak-test/`）保持工具本体纯粹聚焦，不在首屏工具流下堆砌长篇百科文字；深度判据、能力边界与 FAQ 集中维护在 `/about/`（`pageType="article"`，`TechArticle` + `FAQPage`）。
  - 实时诊断卡片、骨架屏、侧边说明框内部严禁使用 `<h2>`/`<h3>`，统一使用 `<p>`，保证全页仅有 1 个 `<h1>`，信息/文章页静态 HTML 为严格的 `H1 -> H2 -> H3` 树。
  - 工具页前后对比记录复用 `client/src/lib/recentHistory.ts`（IndexedDB 单事务保留相邻两次结果）。
- **结构化数据与机器资产**：
  - `client/src/layouts/Layout.astro` 支持 `pageType: 'home' | 'tool' | 'article' | 'static'`，统一输出单块 `@graph` JSON-LD（以 `@id` 关联 `Organization`、`WebSite`、`WebPage`、`WebApplication`、`TechArticle`、`FAQPage`），并支持按页面传入 `ogImage`（默认 `/og-default.png`，`1200×630`），自动同步至 `og:image`、`twitter:image` 与 `primaryImageOfPage`；信息页与文章页 `dateModified` 须与页面及 `client/astro.config.mjs` 的 Sitemap `lastmod` 一致。
  - `client/public/robots.txt` 保持 `Content-Signal: search=yes, ai-input=yes, ai-train=yes` 且不 `Disallow: /api/`；由 Nginx 模板（`deploy/setup/templates/site.conf.tpl`）对 `/api/` 和 `/health` 下发 `X-Robots-Tag "noindex, nofollow" always`。
  - 新增工具页或改动检测能力、判据、接口时，同步更新 `client/astro.config.mjs`、`client/src/pages/about.astro`、`client/public/llms.txt`（遵循 `llmstxt.org`）与 `client/public/llms-full.txt`。
  - FAQ 答案首句必须可独立引用，避免“本页”“系统”等悬空指代。
- **UI 分层与移动端防溢出**：
  - 首屏核心诊断区使用 `.glass-card`，与文章说明页保持清晰定位边界。
  - Flex/Grid 容器内的 `<main>` 与长地址文本（IPv6 / mDNS）必须设置 `min-w-0 max-w-full`（或 `.ip-address` 独立横向滚动），确保 375px 窄屏下无页面级横向溢出（`document.documentElement.scrollWidth <= window.innerWidth`）。
  - 页面不支持的功能按钮（如无截图能力的专项检测页）直接通过 `{#if onScreenshot}` 条件隐藏，不渲染禁用态占位按钮。
  - **Astro 5 条件挂载子组件 CSS 约束**：在 `client:load` 岛组件内仅于条件分支挂载的 Svelte 子组件（如 `DNSFlowStage.svelte`），其 `<style>` 可能被打包为未链接到静态 HTML `<head>` 的孤立文件；此类基础样式统一维护在 `client/src/styles/global.css`。
  - **专项页 2.5D 路径画布约束**：遵循「通栏实测路径画布 → 本次检测结论 → 明细数据」顺序，避免画布与明细间三遍复读；实时采样轮次以不占高度的绝对定位浮层呈现、完结即隐，保证零布局跳动（CLS = 0）、支持 `prefers-reduced-motion`，且 375px 首屏同时容纳画布、判定与核心出口信息。

---

## 4. 部署架构红线（`deploy/`、`docs/deployment.md`）

1. **云主机 NAT 网卡绑定**：在 GCP / AWS / Oracle 等 NAT 机型上，CoreDNS `Corefile` 必须 `bind <内网网卡 IP>`（通过 `ip route get 1.1.1.1` 查看 `src`），不能绑公网 IP（无法监听），也不能绑 `0.0.0.0`（与 `systemd-resolved` `127.0.0.53:53` 冲突）；zone 文件内的 `A` 记录写**公网 IP**。
2. **严禁开启 Cloudflare 橙云（Proxy）**：主域及 `ns1.leak.<域名>` 必须保持 `proxied: false`，否则 Nginx 无法获取真实客户端源 IP，且 `$tcpinfo_rtt` 物理延迟检测失效。
3. **测试/验证域名防抢收录**：任何非正式主站的预览/验证子域，必须在对应 Nginx server 块添加 `add_header X-Robots-Tag "noindex, nofollow" always;`。
4. **防火墙端口**：完整运行需放行 TCP `80` / `443` 以及 TCP/UDP `53`（未放行 53 时主站与 IP/WebRTC/指纹可用，但公网 DNS 泄露探针与 `*.leak.<域名>` 证书签发会超时）。

---

## 5. 常用验证命令

```bash
# 前端类型检查与构建（需 0 errors, 0 warnings）
cd client && npx astro check && npm run build

# 后端单测与 Linux amd64 纯 Go 交叉编译
cd server && go test ./...
cd server && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/detectradar-server ./cmd/server
```
