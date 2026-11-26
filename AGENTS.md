# DetectRadar Agent 工程准则 (`AGENTS.md`)

---

## 1. 协作与变更红线

1. **文档与注释原则**：除非用户明确要求，不要新增、修改或补充代码注释与文档；修改任意代码、配置或接口若会影响其他文件、构建、测试或线上行为，必须第一时间明确提出。
2. **事实优先、极度克制**：所有文案、排障说明、API 描述与调研结论须以仓库真实代码和实测输出为唯一依据，严禁夸大能力或承诺“防封号 / 保证通过风控”；工具本体永远是视觉主角。
3. **可控优先、严禁擅自提交**：未经明确要求，绝不自动执行 `git commit`、`git push`，不擅自修改线上生产配置、DNS 或历史数据；遇不可逆或高影响分岔先给精炼选项确认。
4. **评审顺序**：页面改版或新增核心页按 **Technical SEO → GEO / AI Agent → 排障闭环 → UI/UX** 顺序评审。
5. **眼见为实**：交付前须完成类型检查、静态构建/单测，以及桌面端与 375px 移动端零横向溢出（`scrollWidth <= innerWidth`）核验。

---

## 2. 项目结构与技术栈

- `client/`：Astro 5（`trailingSlash: 'always'`，SSG）+ Svelte 5 + Tailwind CSS v4
- `server/`：Go + Fiber（`CGO_ENABLED=0` 纯 Go，自建权威 NS + DNSTap + UDP STUN + L1/L2/L3 IP 情报）
- `crawler/`：纯 Go 爬虫工具集（`crawler/cloud-ip/` 云厂商与机房 IP 段抓取，`crawler/catalog/` 运营商 ASN/别名/地名增量同步，产出 `hosting.db` / MMDB）
- `macos/`：Swift 6 + AppKit 状态栏客户端（`DetectRadarBar`，纯 Command Line Tools 环境构建，禁用 `.xcstrings`）
- `modules/`：Surge / Stash / Loon 代理面板模块（脚本与各端配置，保留 `#!` 协议头）
- `deploy/`：Docker Compose、Nginx 与 CoreDNS 模板（见 `deploy/README.md`）

---

## 3. 关键不变式与容易踩的坑

### Web / SEO / UI 边界
- 核心检测工具页（`/`、`/dns-leak-test/`、`/webrtc-leak-test/`、`/ipv6-leak-test/`）保持工具本体纯粹聚焦，不在首屏工具流下堆砌长篇百科文字；深度判据、能力边界与 FAQ 集中维护在 `/about/`。
- 全页仅保留 1 个 `<h1>`；实时诊断卡片、骨架屏与侧边栏内部不用 `<h2>`/`<h3>`，统一用 `<p>`。
- 页面不支持的功能按钮直接条件隐藏，不渲染禁用态占位按钮；长地址文本与 Flex/Grid 容器须防 375px 横向溢出。
- Astro `client:load` 岛内条件挂载子组件的基础样式统一放 `client/src/styles/global.css`，避免孤立 CSS 未被静态 HTML 链接。
- `client/public/robots.txt` 不 `Disallow: /api/`，由 Nginx 对 `/api/` 和 `/health` 下发 `X-Robots-Tag: noindex, nofollow`；Nginx 须为 `.md` 下发 `text/markdown; charset=utf-8`。
- 信息/文章页的 `dateModified` 须与 `client/astro.config.mjs` 的 Sitemap `lastmod` 保持一致；改动检测能力、判据或对外接口时，先提醒是否同步更新 `/about/`、`llms.txt`、`llms-full.txt` 与 `SKILL.md`。

### 术语与接口契约
- 泄露检测状态词固定为 `泄露`、`未泄露`、`无法判断`（DNS 另含 `无 DNS 结果`）；无异常总结词统一为 `未见明显异常`。
- `GET /api/v1/ip` 的 `usage_type` 枚举固定为 `isp` / `hosting` / `unknown`（无 `datacenter`），响应固定 `Cache-Control: no-store`，默认走本地 `L1+L2`（显式 `?deep=true` 才触发受每日配额熔断保护的 `L3`）。
- 仅观测当前连接出口，不开放免鉴权查询任意第三方 IP，保护 `L3` 额度及上游免费信誉/BGP 源。

### 部署红线
- NAT 云机上 CoreDNS 须 `bind <内网网卡 IP>`（`ip route get 1.1.1.1` 看 `src`），不可绑公网 IP 或 `0.0.0.0`（冲突 `systemd-resolved`）；zone 记录写公网 IP。
- 主域及 `ns1.leak.<域名>` 严禁开启 Cloudflare 橙云（Proxy），否则丢真实客户端 IP 与 `$tcpinfo_rtt`。
- 非正式主站的测试/验证子域须在对应 Nginx server 块添加 `X-Robots-Tag: noindex, nofollow`。
- 完整运行须放行 TCP `80`/`443`、TCP/UDP `53` 与 UDP `3478`（自建同域 WebRTC STUN）。

---

## 4. 常用验证命令

```bash
# 前端类型检查与构建（需 0 errors, 0 warnings）
cd client && npx astro check && npm run build

# 后端单测与 Linux amd64 纯 Go 交叉编译
cd server && go test ./...
cd server && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/detectradar-server ./cmd/server

# 爬虫（云 IP + 网络目录）格式检查与单测
cd crawler && test -z "$(gofmt -l .)" && go vet ./... && go test ./...

# macOS 菜单栏客户端单测与打包（Command Line Tools 环境）
cd macos && swift test --disable-xctest -Xswiftc -plugin-path -Xswiftc /Library/Developer/CommandLineTools/usr/lib/swift/host/plugins/testing
cd macos && ./scripts/build-app.sh && codesign --verify --deep --strict dist/DetectRadarBar.app
```
