# syntax=docker/dockerfile:1
# 构建上下文 = 仓库根目录：docker build -f deploy/images/crawler.Dockerfile .
FROM golang:1.24-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY crawler/go.mod crawler/go.sum ./
RUN go mod download
COPY crawler/ ./
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/harrisonwang/detect-radar/crawler/cloud-ip/internal/crawler.Version=${VERSION}" \
      -o /out/cloud-ip-crawler ./cloud-ip/cmd/cloud-ip-crawler && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/harrisonwang/detect-radar/crawler/catalog/internal/sync.Version=${VERSION}" \
      -o /out/catalog-crawler ./catalog/cmd/catalog-crawler

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/cloud-ip-crawler /usr/local/bin/cloud-ip-crawler
COPY --from=build /out/catalog-crawler /usr/local/bin/catalog-crawler
COPY data/catalog_seed.json /usr/local/share/detectradar/catalog_seed.json
VOLUME /data
ENTRYPOINT ["/usr/local/bin/cloud-ip-crawler"]
CMD ["--db", "/data/hosting.db"]
