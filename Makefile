# =============================================================================
# MagicStream Makefile
# =============================================================================
# Makefile 的设计原则：
#   1. 所有目标都用 .PHONY 声明（因为不是真实文件名，而是任务名称）
#   2. 每个目标有简要注释描述其功能
#   3. 尽量用一行命令完成任务（但为了可读性可以换行）
#   4. 不假设特定的操作系统或 shell（但依赖 bash 语法的地方要注明）
#
# 使用方式：
#   make dev    启动开发环境（PG + Redis + MagicStream）
#   make test   运行所有单元测试
#   make clean  清除所有数据，从头开始
# =============================================================================

.PHONY: all dev up down build test bench cover lint fmt clean

# 默认目标：代码检查 + 编译
all: lint test build

# =========================================================================
# 开发
# =========================================================================

# dev: 启动 PostgreSQL + Redis，等它们就绪后再启动 Go 应用。
# 为什么等 3 秒？Docker Compose 的 depends_on 只能等容器启动，
# 不能等容器内的服务就绪。healthcheck 要持续几轮才能确认。
# 3 秒是一个折中值——大多数情况下 PG 和 Redis 都能在 3 秒内就绪。
dev:
	docker compose up -d postgres redis
	@echo "等待 PostgreSQL 和 Redis 就绪..."
	@sleep 3
	go run ./cmd/magicstream

# =========================================================================
# Docker
# =========================================================================

# up: 用 Docker Compose 构建并启动所有服务。
# --build 每次都重新构建镜像（确保包含最新代码变更）。
# MagicStream 本身也会被构建进 Docker 并使用 Dockerfile 中的 ENTRYPOINT 启动。
up:
	docker compose up -d --build

# down: 停止并移除所有容器和网络。
# 不带 -v 参数，数据卷会被保留（pgdata 和 redisdata 不会丢失）。
# 下次 docker-compose up 时数据还在，就像暂停和恢复一样。
down:
	docker compose down

# build: 只构建 Docker 镜像，不启动。
# 用于验证 Dockerfile 是否正确、提前下载依赖。
build:
	docker build -t magicstream:latest .

# =========================================================================
# 测试
# =========================================================================

# test: 运行所有单元测试。
# -count=1 禁用 Go test 的缓存机制，保证每次都重新执行测试。
# 默认情况下，如果代码和测试都没变，go test 会返回缓存结果（显示 (cached)）。
# -timeout 60s 防止某个测试死锁卡住整个测试流程。
test:
	go test ./... -count=1 -v -timeout 60s

# bench: 运行所有 benchmark。
# -benchmem 额外输出每次操作的内存分配次数（allocs/op）和字节数（B/op）。
# 这两个指标对性能调优至关重要——分配越少，GC 压力越小。
bench:
	go test ./... -bench=. -benchmem -timeout 120s

# cover: 运行测试并生成 HTML 覆盖率报告。
# -coverprofile=coverage.out 生成覆盖率原始数据。
# go tool cover -html 把原始数据渲染为 HTML，浏览器中可以看到每行是否被测试覆盖。
cover:
	go test ./... -coverprofile=coverage.out -timeout 60s
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# =========================================================================
# 代码质量
# =========================================================================

# lint: 运行 golangci-lint 静态检查。
# golangci-lint 集成了多个 linter（govet, staticcheck, errcheck, ineffassign 等），
# 一次运行检查多种问题。
# 需要先安装：go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
lint:
	golangci-lint run ./...

# fmt: 格式化所有 Go 代码。
# gofmt -w 直接写入文件（-w = write）。
# 注意：不处理 import 排序，如需自动排序可以用 goimports。
fmt:
	gofmt -w ./cmd ./internal

# =========================================================================
# 清理
# =========================================================================

# clean: 彻底清理一切。
# docker compose down -v：
#   -v 删除所有数据卷（pgdata + redisdata），数据库和缓存完全清空。
# rm -rf ./data：删除本地的视频/转码/上传文件。
# rm -f coverage.*：删除测试覆盖率文件。
clean:
	docker compose down -v
	rm -rf ./data/videos/* ./data/live/* ./data/uploads/*
	rm -f coverage.out coverage.html
	rm -f magicstream magicstream.exe
	@echo "清理完成——所有数据已删除，数据库已重置"