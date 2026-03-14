# ==================== 构建阶段 ====================

# 阶段1: 构建前端
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

COPY easy-strm-front/package*.json ./
RUN npm install

COPY easy-strm-front/ ./
RUN npm run build

# 阶段2: 构建后端
FROM golang:1.23-alpine AS backend-builder

WORKDIR /app/backend

RUN apk add --no-cache git

COPY easy-strm/go.mod easy-strm/go.sum* ./
RUN go mod download || go mod tidy

COPY easy-strm/ ./
ENV GOTOOLCHAIN=auto
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o easy-strm .

# ==================== 运行阶段 ====================
FROM alpine:3.19

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
RUN mkdir -p /app/logs

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
    REDIS_PASSWORD=""

# 启动supervisor管理进程
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
