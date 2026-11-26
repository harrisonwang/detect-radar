# syntax=docker/dockerfile:1
# Build from crawler/ context: docker build -f cloud-ip/Dockerfile .
FROM golang:1.24-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/harrisonwang/detect-radar/crawler/cloud-ip/internal/crawler.Version=${VERSION}" \
      -o /out/cloud-ip-crawler ./cloud-ip/cmd/cloud-ip-crawler

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/cloud-ip-crawler /usr/local/bin/cloud-ip-crawler
VOLUME /data
ENTRYPOINT ["/usr/local/bin/cloud-ip-crawler"]
CMD ["--db", "/data/hosting.db"]
