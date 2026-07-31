# DetectRadar 自托管部署

一套 `docker compose up -d` 拉起完整 DetectRadar：前端 + Go API + CoreDNS 信标权威 DNS + 证书自动签发 + 云 IP 段爬虫。DNS 记录、NS 委派、TLS 证书全部通过 Cloudflare API 自动完成，唯一需要在页面上做的事是**创建一次 API Token**。

## 架构

```
                    ┌────────────────────────── 你的服务器（host 网络） ─────────────────────────┐
 浏览器 ──443/80──▶ │ web (nginx+前端)  ──127.0.0.1:8080──▶ server (Go API)                      │
                    │        │ X-TCP-RTT($tcpinfo_rtt)              ▲ dnstap 127.0.0.1:6000      │
 递归解析器 ──53──▶ │ coredns (leak.<域名> 权威)  ────────────────┘                              │
                    │ acme (证书签发/续期)   crawler (云 IP 段每日更新)   setup (一次性初始化)   │
                    └───────────────────────────────────────────────────────────────────────────┘
```

整套服务跑在 `network_mode: host` 上，这是硬约束而非偏好：源 IP 保真、nginx `$tcpinfo_rtt` 物理测距、CoreDNS 对公网答 53 三件事在桥接/rootless 网络下都会静默失真。

## 前置条件

- Linux 服务器（amd64 / arm64），**独占公网 IP**，53、80、443 端口空闲
  - `systemd-resolved` 不冲突（它只占 `127.0.0.53`）；`dnsmasq`/`named` 占用 `0.0.0.0:53` 则需先释放
  - **云厂商防火墙/安全组要放行入站 TCP 80/443 + TCP/UDP 53**（腾讯云/阿里云等默认只开 80/22，53 和 443 被拦时的症状：主站能 301 跳转但 HTTPS 超时、`*.leak` 证书签发卡在 DNS 验证超时）
- root 模式的 Docker + Compose v2（**不支持 rootless Docker**，它会丢源 IP）
- 域名已托管在 Cloudflare（NS 指向 Cloudflare）

## 快速开始

1. 创建 Cloudflare API Token（唯一的页面操作）：
   [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) → 创建令牌 → 模板「编辑区域 DNS」→ 区域资源限定到你的域名 zone（权限即 Zone→DNS→Edit + Zone→Zone→Read）。

2. 下载编排文件并填参数：

```bash
mkdir detect-radar && cd detect-radar
curl -fsSLO https://raw.githubusercontent.com/harrisonwang/detect-radar/main/deploy/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/harrisonwang/detect-radar/main/deploy/.env.example -o .env
chmod 600 .env   # 里面要放 CF Token
vi .env          # 必填仅 3 项：DOMAIN / CF_Token / ACME_EMAIL
```

3. 启动：

```bash
docker compose up -d
```

首次启动会依次自动完成：探测公网 IP 与经纬度 → 在 Cloudflare 创建 `A`（主站）、`A ns1.leak.<域名>`（glue）、`NS leak.<域名>`（信标 zone 委派）→ 渲染 nginx/CoreDNS/后端配置 → 生成占位证书拉起 nginx → acme 容器经 DNS-01 签出真证书并自动热替换 → 爬虫开始首轮云 IP 段抓取。新委派的 DNS 生效可能需要几分钟，acme 容器会自动重试。

## 验证

```bash
dig NS leak.你的域名 @1.1.1.1
```

```bash
dig A test.leak.你的域名 @你的服务器IP
```

```bash
curl https://你的域名/health
```

最后用真实浏览器（别在服务器本机 curl）做一次完整扫描，确认：报告显示的出口 IP 是你客户端的真实 IP（若出现 `172.x` 内网地址说明网络模式被改错了）、RTT 数值量级合理、DNS 泄露检测能出结果。

## 参数与运维

| 操作 | 命令 |
|---|---|
| 修改 `.env` 后生效 | `docker compose up -d && docker compose restart server coredns web` |
| 升级镜像 | `docker compose pull && docker compose up -d` |
| 更新地理库 | 替换 `radar_data` 卷里的 mmdb/xdb 后 `docker compose restart server`（mmdb 被 mmap，必须重启） |
| 看证书状态 | `docker compose exec acme acme.sh --list` |
| 数据备份 | `radar_data` 卷（hosting.db、scans.jsonl、地理库）+ `acme_state` 卷 |

全部参数见 [.env.example](.env.example)。第三方情报源 key（ipapi.is 等）都可留空，只影响判定准确率，不影响启动。

## 镜像

| 镜像 | 内容 |
|---|---|
| `detect-radar-server` | Go API，内置 DB-IP Lite（CC BY 4.0）与 ip2region 地理库，首启播种到数据卷 |
| `detect-radar-web` | nginx + Astro 前端产物，inotify 自动热载新证书 |
| `detect-radar-setup` | 初始化器：Cloudflare API、配置渲染、占位证书 |
| `cloud-ip-crawler` | 云厂商 IP 段爬虫（[独立仓库](https://github.com/harrisonwang/cloud-ip-crawler)） |

默认从 `ghcr.io/harrisonwang` 拉取；国内机器把 `.env` 里的 `REGISTRY` 切到 `registry.cn-shenzhen.aliyuncs.com/harrisonwang`（CoreDNS、acme.sh 官方镜像也已同步到该 ACR，见 `.env.example` 注释）。

## 已知边界

- **Cloudflare 橙云（代理）必须关闭**：setup 会强制把相关记录设为「仅 DNS」并在检测到橙云时告警。经 CF 反代后源 IP 与 RTT 全部失真，整个产品失去意义。
- 前端 canonical/sitemap 默认指向 detectradar.com：不影响功能；在意 SEO 标记的话本地构建 web 镜像并传 `--build-arg PUBLIC_SITE_URL=https://你的域名`。
- 服务器地理定位默认取自 ip-api.com：机房 IP 定位偶有偏差，RTT 校验异常时手工设置 `SERVER_LAT`/`SERVER_LON`。
- 首轮爬虫完成前（约几分钟），扫描结果里的云厂商归属层可能缺失。
