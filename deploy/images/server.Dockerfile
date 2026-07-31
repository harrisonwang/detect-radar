# syntax=docker/dockerfile:1
# 构建上下文 = 仓库根目录：docker build -f deploy/images/server.Dockerfile .
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/detectradar-server ./cmd/server

# 地理库播种层：DB-IP Lite（免 key、月更、CC BY 4.0）+ ip2region（CN 权威层）。
# 烤进镜像而非运行时下载：国内环境从 ACR 拉镜像远快于装机时直连 db-ip/GitHub。
FROM alpine:3.20 AS geodata
ARG DBIP_MONTH=2026-07
ARG DBIP_MONTH_PREV=2026-06
RUN set -e; mkdir -p /seed/ip2region-cn; \
    fetch() { wget -q "https://download.db-ip.com/free/dbip-${1}-lite-${DBIP_MONTH}.mmdb.gz" -O "/tmp/${1}.gz" \
           || wget -q "https://download.db-ip.com/free/dbip-${1}-lite-${DBIP_MONTH_PREV}.mmdb.gz" -O "/tmp/${1}.gz"; \
           gunzip -c "/tmp/${1}.gz" > "/seed/dbip-${1}-lite.mmdb"; }; \
    fetch city; fetch asn; \
    wget -q https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ip2region_v4.xdb -O /seed/ip2region-cn/ip2region_v4.xdb \
      || wget -q https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ip2region.xdb -O /seed/ip2region-cn/ip2region_v4.xdb; \
    wget -q https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ip2region_v6.xdb -O /seed/ip2region-cn/ip2region_v6.xdb \
      || { echo "ip2region v6 库不可用，跳过（仅影响 CN IPv6 城市精度）"; rm -f /seed/ip2region-cn/ip2region_v6.xdb; }

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/detectradar-server /app/detectradar-server
COPY --from=geodata /seed /app/seed
COPY deploy/server/entrypoint.sh /app/entrypoint.sh
ENTRYPOINT ["/bin/sh", "/app/entrypoint.sh"]
