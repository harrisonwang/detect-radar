VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/harrisonwang/detect-radar/cloud-ip-crawler/internal/crawler.Version=$(VERSION)

.PHONY: build test lint run clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o cloud-ip-crawler ./cmd/cloud-ip-crawler

test:
	go test ./...

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...

run: build
	./cloud-ip-crawler --db cloud-ip.db

clean:
	rm -f cloud-ip-crawler cloud-ip.db cloud-ip.csv*
