# Easy-STRM 部署指南

## 快速部署

### 方式一：一键部署（推荐）

```bash
# 1. 进入部署目录
cd deploy

# 2. 复制配置文件
cp .env.example .env

# 3. 编辑配置文件
nano .env  # 或 vi .env

# 4. 运行部署脚本
chmod +x deploy.sh
./deploy.sh
```

### 方式二：手动部署

```bash
# 1. 复制配置文件
cp .env.example .env

# 2. 编辑配置（重要！）
nano .env

# 3. 启动服务
docker-compose up -d

# 4. 查看日志
docker-compose logs -f
```

## 配置说明

### 必须修改的配置

| 配置项 | 说明 | 示例 |
|--------|------|------|
| `SERVER_URL` | 服务器访问地址 | `http://192.168.1.100:80` 或 `https://strm.example.com` |
| `JWT_SECRET` | JWT密钥（随机字符串） | `my_random_secret_key_12345` |
| `PG_PASSWORD` | 数据库密码 | `your_secure_password` |

### 可选配置

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `APP_PORT` | 应用端口 | `80` |
| `PG_PORT_EXPOSE` | PostgreSQL 对外端口 | `5432` |
| `REDIS_PORT_EXPOSE` | Redis 对外端口 | `6379` |
| `REDIS_PASSWORD` | Redis 密码 | (空) |

## 默认账号

- 用户名：`admin`
- 密码：`admin123`

**请在首次登录后立即修改密码！**

## 常用命令

```bash
# 启动服务
docker-compose up -d

# 停止服务
docker-compose down

# 重启服务
docker-compose restart

# 查看日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f easy-strm

# 更新镜像
docker pull joshuabarrett2117/easy-strm:latest
docker-compose up -d

# 清理数据（危险操作！）
docker-compose down -v
```

## 端口说明

| 服务 | 内部端口 | 对外端口（可配置） |
|------|----------|-------------------|
| Easy-STRM | 80 | `APP_PORT` (默认 80) |
| PostgreSQL | 5432 | `PG_PORT_EXPOSE` (默认 5432) |
| Redis | 6379 | `REDIS_PORT_EXPOSE` (默认 6379) |

## 数据持久化

数据存储在 Docker volumes 中：

- `postgres_data` - PostgreSQL 数据
- `redis_data` - Redis 数据
- `app_logs` - 应用日志

## 反向代理配置（可选）

如果需要使用 Nginx 或 Caddy 作为反向代理：

### Nginx 配置示例

```nginx
server {
    listen 80;
    server_name strm.example.com;

    location / {
        proxy_pass http://127.0.0.1:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Caddy 配置示例

```
strm.example.com {
    reverse_proxy localhost:80
}
```

## 故障排除

### 服务无法启动

```bash
# 检查日志
docker-compose logs easy-strm

# 检查数据库连接
docker-compose exec postgres pg_isready -U easystrm

# 检查 Redis 连接
docker-compose exec redis redis-cli ping
```

### 无法访问服务

1. 检查防火墙是否开放端口
2. 检查 `SERVER_URL` 配置是否正确
3. 检查容器是否正常运行：`docker-compose ps`

### 数据库连接失败

1. 确认 PostgreSQL 容器已启动
2. 检查数据库密码配置是否一致
3. 查看数据库日志：`docker-compose logs postgres`
