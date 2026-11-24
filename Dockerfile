# syntax=docker/dockerfile:1.7
# ==================== 构建阶段 ====================

# 阶段1: 构建前端
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

# 可通过 --build-arg NPM_REGISTRY=地址 覆盖 npm 源
ARG NPM_REGISTRY=https://registry.npmmirror.com

COPY easy-strm-front/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm config set fetch-retries 5 && \
    npm config set fetch-retry-factor 2 && \
    npm config set fetch-retry-mintimeout 10000 && \
    npm config set fetch-retry-maxtimeout 120000 && \
    npm ci --ignore-scripts --prefer-offline --no-audit --no-fund --registry="${NPM_REGISTRY}" && \
    (npm rebuild esbuild --foreground-scripts || (sleep 3 && npm rebuild esbuild --foreground-scripts))

COPY easy-strm-front/ ./
RUN npm run build

# 阶段2: 构建后端
FROM golang:1.25-alpine AS backend-builder

WORKDIR /app/backend

RUN apk add --no-cache git

# 可通过 --build-arg GOPROXY=地址 覆盖 Go 模块代理
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY easy-strm/go.mod easy-strm/go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY easy-strm/ ./
# 不在构建期执行 go mod tidy：以仓库中的 go.mod/go.sum 为准，保证构建可复现
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly \
    go build -trimpath -ldflags="-s -w" -o easy-strm .

# ==================== 运行阶段 ====================
FROM alpine:3.22

LABEL maintainer="easy-strm"
LABEL description="Easy-STRM - 115网盘文件直链获取工具"

# 安装必要的工具
RUN apk add --no-cache nginx supervisor tzdata

# 设置时区
ENV TZ=Asia/Shanghai

# 创建应用目录
WORKDIR /app

# 从构建阶段复制后端程序
COPY --from=backend-builder /app/backend/easy-strm /app/easy-strm

# 从构建阶段复制前端构建产物
COPY --from=frontend-builder /app/frontend/dist /app/frontend/dist

# 复制nginx配置
COPY docker/nginx.conf /etc/nginx/http.d/default.conf

# 复制supervisor配置
COPY docker/supervisord.conf /etc/supervisord.conf

# 复制启动脚本
COPY docker/start.sh /app/start.sh
RUN chmod +x /app/start.sh

# 创建日志目录
RUN mkdir -p /app/logs /var/log/supervisor /var/log/nginx

# 暴露端口（前端端口）
EXPOSE 80

# 设置环境变量默认值（用户可通过docker run -e 或 docker-compose覆盖）
ENV SERVER_URL="http://localhost:80" \
    JWT_SECRET="easy_strm_jwt_secret" \
    PG_HOST="postgres" \
    PG_PORT="5432" \
    PG_DATABASE="easy_strm" \
    PG_USER="postgres" \
    PG_PASSWORD="postgres" \
    REDIS_HOST="redis" \
    REDIS_PORT="6379" \
    REDIS_PASSWORD="" \
    REDIS_DB="0" \
    LOG_OUTPUT_TYPE="file" \
    LOG_LEVEL="INFO"

# 健康检查：nginx 静态页可访问即视为容器存活
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1/ || exit 1

# 启动supervisor管理进程
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
