# syntax=docker/dockerfile:1
# 构建上下文 = 仓库根目录：docker build -f deploy/images/web.Dockerfile .
FROM node:22-alpine AS fe
WORKDIR /build
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/ ./
# 同源部署：API base 留空走 /api/v1，前端产物域名无关（信标域名由后端下发）。
# PUBLIC_SITE_URL 只影响 canonical/sitemap；自建域名要正确 SEO 标记就本地构建时覆盖此参
ARG PUBLIC_SITE_URL=https://detectradar.com
ENV PUBLIC_SITE_URL=${PUBLIC_SITE_URL} \
    PUBLIC_API_BASE=
RUN npm run build

FROM nginx:stable-alpine
# inotify-tools：证书/配置卷变化时自动 nginx -s reload（acme 续期后无需人工介入）
RUN apk add --no-cache inotify-tools
COPY --from=fe /build/dist /usr/share/nginx/site
COPY deploy/web/entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
