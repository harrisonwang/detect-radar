VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/harrisonwang/detect-radar/crawler/cloud-ip/internal/crawler.Version=$(VERSION)

.PHONY: build test lint run clean

build:
	cd .. && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o cloud-ip/cloud-ip-crawler ./cloud-ip/cmd/cloud-ip-crawler

test:
	cd .. && go test ./cloud-ip/...

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	cd .. && go vet ./cloud-ip/...

run: build
	./cloud-ip-crawler --db cloud-ip.db

clean:
	rm -f cloud-ip-crawler cloud-ip.db cloud-ip.csv*
