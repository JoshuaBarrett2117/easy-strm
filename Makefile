# Easy-STRM 本地开发命令入口（替代原 start/stop/restart.bat 与 easy-strm/run_tests.bat）
# Windows 无 make 时，按下方各目标中的命令在对应目录手工执行即可。

BACKEND_DIR  := easy-strm
FRONTEND_DIR := easy-strm-front
FRONTEND_PORT ?= 3001
IMAGE ?= easy-strm:dev

.PHONY: help deps dev-backend dev-frontend test test-backend test-frontend lint lint-frontend vet build build-backend build-frontend docker-build clean

help: ## 列出可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

deps: ## 安装前端依赖并下载 Go 模块
	cd $(FRONTEND_DIR) && npm ci
	cd $(BACKEND_DIR) && go mod download

dev-backend: ## 启动后端（:8082，需先按 .env.example 准备 PG/Redis 环境变量）
	cd $(BACKEND_DIR) && go run .

dev-frontend: ## 启动前端 Vite 开发服务器（:3001，/api 代理到 :8082）
	cd $(FRONTEND_DIR) && npm run dev -- --host 0.0.0.0 --port $(FRONTEND_PORT) --strictPort

test: test-backend test-frontend ## 运行后端与前端单元测试

test-backend: vet ## go test ./...
	cd $(BACKEND_DIR) && go test ./...

test-frontend: ## 前端单元测试（node --test + vitest 组件测试）
	cd $(FRONTEND_DIR) && npm test

lint: vet lint-frontend ## 后端 go vet + 前端 ESLint

lint-frontend: ## 前端 ESLint（0 error 才通过，warning 允许）
	cd $(FRONTEND_DIR) && npm run lint

vet: ## go vet ./...
	cd $(BACKEND_DIR) && go vet ./...

build: build-backend build-frontend ## 构建后端与前端

build-backend: ## go build ./...
	cd $(BACKEND_DIR) && go build ./...

build-frontend: ## npm run build
	cd $(FRONTEND_DIR) && npm run build

docker-build: ## 本地构建镜像（默认标签 easy-strm:dev）
	docker build -t $(IMAGE) .

clean: ## 清理前端构建产物与 Go 测试缓存
	rm -rf $(FRONTEND_DIR)/dist
	cd $(BACKEND_DIR) && go clean -testcache
