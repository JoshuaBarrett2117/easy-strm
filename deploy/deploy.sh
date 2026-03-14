#!/bin/bash

# Easy-STRM 快速部署脚本

set -e

echo "=========================================="
echo "  Easy-STRM 部署脚本"
echo "=========================================="

# 检查 Docker 和 Docker Compose
if ! command -v docker &> /dev/null; then
    echo "错误: Docker 未安装，请先安装 Docker"
    exit 1
fi

if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "错误: Docker Compose 未安装，请先安装 Docker Compose"
    exit 1
fi

# 检查 .env 文件
if [ ! -f ".env" ]; then
    echo "未找到 .env 文件，正在从模板创建..."
    cp .env.example .env
    echo "已创建 .env 文件，请编辑配置后重新运行此脚本"
    echo ""
    echo "重要配置项："
    echo "  1. SERVER_URL - 修改为你的实际访问地址"
    echo "  2. JWT_SECRET - 修改为随机字符串"
    echo "  3. PG_PASSWORD - 修改数据库密码"
    echo ""
    echo "编辑命令: nano .env 或 vi .env"
    exit 0
fi

# 拉取最新镜像
echo "正在拉取最新镜像..."
docker pull joshuabarrett2117/easy-strm:latest

# 启动服务
echo "正在启动服务..."
if docker compose version &> /dev/null; then
    docker compose up -d
else
    docker-compose up -d
fi

echo ""
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
echo ""
echo "访问地址: http://localhost:${APP_PORT:-80}"
echo ""
echo "默认管理员账号: admin"
echo "默认管理员密码: admin123"
echo ""
echo "常用命令："
echo "  查看日志: docker-compose logs -f"
echo "  停止服务: docker-compose down"
echo "  重启服务: docker-compose restart"
echo ""
