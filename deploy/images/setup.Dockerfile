# syntax=docker/dockerfile:1
# 构建上下文 = 仓库根目录：docker build -f deploy/images/setup.Dockerfile .
FROM alpine:3.20
RUN apk add --no-cache curl jq openssl gettext bind-tools iproute2
COPY deploy/setup/templates /templates
COPY deploy/setup/assets /assets
COPY deploy/setup/entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
