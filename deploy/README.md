# 使用 Docker 部署 DetectRadar

DetectRadar 可以通过 Docker Compose 启动网站、API、DNS 服务和 HTTPS 证书服务。

## 部署前准备

需要准备：

- Linux 服务器和公网 IP；
- TCP 80、443，以及 TCP/UDP 53 端口；
- 一个托管在 Cloudflare 的域名；
- Docker Engine 和 Docker Compose；
- 一个 Cloudflare API 令牌。

API 令牌需要具备以下权限：

- Zone → DNS → Edit
- Zone → Zone → Read

项目使用 Docker 的主机网络模式（host）。这样 API 才能获取真实客户端 IP，DNS 服务也能直接监听 53 端口。请使用 root 权限运行 Docker，不要改成 rootless Docker 或 bridge 网络。

## 开始部署

```bash
mkdir detect-radar
cd detect-radar

curl -fsSLO https://raw.githubusercontent.com/harrisonwang/detect-radar/main/deploy/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/harrisonwang/detect-radar/main/deploy/.env.example -o .env

chmod 600 .env
vi .env
docker compose up -d
```

`.env` 中必须填写：

```dotenv
DOMAIN=你的域名
CF_Token=Cloudflare API 令牌
ACME_EMAIL=你的邮箱
```

首次启动会自动完成以下工作：

- 创建或更新 Cloudflare DNS 记录；
- 生成 API、nginx 和 DNS 配置；
- 启动网站和 API；
- 由 acme 服务申请 HTTPS 证书；
- 启动云厂商 IP 数据更新服务。

首次申请证书和等待 DNS 生效可能需要几分钟。

国内机器可按 [`.env.example`](.env.example) 中的注释配置镜像源。

## 检查服务

查看容器状态：

```bash
docker compose ps
```

查看 API 日志：

```bash
docker compose logs --tail=100 server
```

检查网站：

```bash
curl https://你的域名/health
```

检查 DNS：

```bash
dig NS leak.你的域名 @1.1.1.1
dig A test.leak.你的域名 @你的服务器IP
```

确认服务正常后，用浏览器打开网站并完成一次扫描。

## 更新配置和镜像

修改 `.env` 后执行：

```bash
docker compose up -d
docker compose restart server coredns web
```

更新镜像：

```bash
docker compose pull
docker compose up -d
```

查看证书状态：

```bash
docker compose exec acme acme.sh --list
```
