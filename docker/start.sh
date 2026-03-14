#!/bin/sh

# 启动脚本 - 用于初始化和启动服务

echo "=========================================="
echo "  Easy-STRM 启动中..."
echo "=========================================="

# 打印环境变量配置信息
echo ""
echo "配置信息:"
echo "  - Server URL: ${SERVER_URL}"
echo "  - PostgreSQL: ${PG_HOST}:${PG_PORT}/${PG_DATABASE}"
echo "  - Redis: ${REDIS_HOST}:${REDIS_PORT}"
echo ""

# 创建必要的目录
mkdir -p /app/logs
mkdir -p /var/log/supervisor

# 启动supervisor
exec /usr/bin/supervisord -c /etc/supervisord.conf
