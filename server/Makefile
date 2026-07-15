.PHONY: build run dev clean test

# 构建
build:
	go build -o detect-radar cmd/server/main.go

# 运行
run: build
	./detect-radar

# 开发模式（需要air工具）
dev:
	air

# 清理
clean:
	rm -f detect-radar
	rm -rf tmp/

# 测试
test:
	go test -v ./...

# 安装依赖
deps:
	go mod download
	go mod tidy

# 格式化代码
fmt:
	go fmt ./...

# 代码检查：gofmt 零欠账 + go vet 必过；装了 golangci-lint 则一并跑
lint:
	@fmtout=$$(gofmt -l .); if [ -n "$$fmtout" ]; then echo "gofmt 不干净:"; echo "$$fmtout"; exit 1; fi
	go vet ./...
	@command -v golangci-lint >/dev/null && golangci-lint run || true

# 帮助
help:
	@echo "可用命令:"
	@echo "  make build  - 编译项目"
	@echo "  make run    - 编译并运行"
	@echo "  make dev    - 开发模式（热重载）"
	@echo "  make clean  - 清理构建文件"
	@echo "  make test   - 运行测试"
	@echo "  make deps   - 安装依赖"
	@echo "  make fmt    - 格式化代码"
	@echo "  make lint   - 代码检查"
