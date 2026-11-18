# DetectRadar

**网络环境一致性检测工具。** 核对当前出口 IP 与浏览器的时区、语言和环境特征，同时检查 WebRTC、DNS 和 IPv6 泄露，生成诊断报告和雷达图。

[在线体验](https://detectradar.com/) · [隐私政策](https://detectradar.com/privacy/)

![DetectRadar：网络环境一致性检测](client/public/og-default.png)

## 核心功能

- 核对浏览器时区、系统语言与当前出口 IP 所在地区是否一致。
- 检查 WebRTC、DNS 和 IPv6 是否暴露了与当前出口不同的网络信息。
- 展示当前出口 IP 的归属地、ASN、运营商、网络类型和信誉信息。
- 检查 Canvas、Audio、WebGL 等浏览器环境特征及自动化标记。
- 将检测结果整理成问题清单、雷达图和分享卡片。

打开网页即可开始检测，无需安装或注册。

## 工作原理

浏览器采集环境信息和网络泄露信号，后端结合出口 IP 完成分析和评分。

DNS 检测结果会稍后返回。无法完成的检测会标为“未检测”。

## 能力边界

- 检测结果仅供参考，不能保证账号安全或避免平台关联。
- IP 信誉、开放端口等部分信息来自第三方数据源，可能存在延迟或误差。
- 浏览器设置、扩展和网络策略可能影响检测。

## 数据与隐私

检测记录和反馈用于排查问题与改进检测。分享图片在浏览器本地生成，IP 自动脱敏。详见[隐私政策](https://detectradar.com/privacy/)。

## 项目结构

| 目录 | 说明 |
| --- | --- |
| `client/` | Astro + Svelte 5 前端，负责采集信息和展示报告 |
| `server/` | Go + Fiber 后端，负责出口 IP 信息查询、一致性判断、泄露复核和评分 |

机房识别所需的云厂商 IP 段由独立项目 [cloud-ip-crawler](https://github.com/harrisonwang/cloud-ip-crawler) 生成。

## 快速开始

### Docker 部署

准备一台能从公网访问的 Linux 服务器，并放行 TCP 80、443 和 TCP/UDP 53 端口。域名需要托管在 Cloudflare，还需要一个具备 Zone DNS Edit 和 Zone Read 权限的 API 令牌（配置项名为 `CF_Token`）。

```bash
mkdir detect-radar
cd detect-radar

curl -fsSLO https://raw.githubusercontent.com/harrisonwang/detect-radar/main/deploy/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/harrisonwang/detect-radar/main/deploy/.env.example -o .env

chmod 600 .env
vi .env    # 填写 DOMAIN、CF_Token 和 ACME_EMAIL
docker compose up -d
```

首次启动会自动创建 DNS 记录、生成配置，并由 acme 服务申请 HTTPS 证书，通常需要几分钟。国内服务器可按 `.env.example` 中的注释配置镜像源。完整的检查和维护方法见 [deploy/README.md](deploy/README.md)。

### 本地开发

后端运行前需要准备 `server/data/` 下的地理与 ASN 数据库，以及机房识别数据库。

```bash
# 后端
cd server
cp configs/config.yaml.example configs/config.yaml
make run

# 前端（另开一个终端）
cd client
npm install
npm run dev
```

前端默认调用 `http://127.0.0.1:8080`，可以通过 `PUBLIC_API_BASE` 修改。

### 邮件通知

用户提交反馈后，通过 Cloudflare 发送邮件通知。

先在 [Email Sending](https://developers.cloudflare.com/email-service/get-started/send-emails/) 验证发信域名，创建具有 **Email Sending: Edit** 权限的 API Token；再用 [Email Routing](https://developers.cloudflare.com/email-service/get-started/route-emails/) 将收件地址转发到已验证的日常邮箱。

填写 `server/configs/config.yaml` 中的 `mail` 配置：

```yaml
mail:
  account_id: "你的 Cloudflare Account ID"
  api_token: "你的 API Token"
  from: noreply@detectradar.com
  to: feedback@detectradar.com
```

保存后重启后端。凭证均留空时，仅保存反馈。
