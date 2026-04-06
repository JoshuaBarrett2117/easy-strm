# MediaManager 模块测试执行指南

## 一、测试文件列表

### 1. DAO 层测试文件
- **文件路径**：`easy-strm/internal/dao/media_source_dao_test.go`
- **测试范围**：媒体源数据访问层
- **测试用例数**：14 个
- **覆盖功能**：
  - 创建媒体源（本地/云端）
  - 更新媒体源
  - 删除媒体源
  - 查询媒体源（单个/列表/按类型/按状态）
  - 参数校验

### 2. Service 层测试文件
- **文件路径**：`easy-strm/internal/service/media_source_service_test.go`
- **测试范围**：媒体源业务逻辑层
- **测试用例数**：12 个
- **覆盖功能**：
  - 媒体源 CRUD 业务逻辑
  - 参数校验（名称/路径/类型/账号ID）
  - 路径有效性校验
  - 115 账号有效性校验

### 3. 文件操作测试文件
- **文件路径**：`easy-strm/internal/service/file_operation_service_test.go`
- **测试范围**：文件浏览与操作
- **测试用例数**：21 个
- **覆盖功能**：
  - 文件列表获取（分页/排序/过滤/搜索）
  - 文件详情获取
  - 本地文件操作（移动/复制/删除/重命名）
  - 批量文件操作
  - 异常场景处理
  - 边界条件测试
  - 性能测试

---

## 二、测试执行命令

### 1. 快速执行（Windows）

```powershell
# 进入项目目录
cd c:\Users\a3875\Documents\code\easy-strm\easy-strm

# 执行所有测试
.\run_tests.bat
```

### 2. 手动执行（推荐）

#### 2.1 执行 DAO 层测试
```powershell
# 执行所有 DAO 层测试
go test -v -race ./internal/dao/... -run "MediaSource"

# 执行单个测试用例
go test -v -race ./internal/dao/... -run "TestCreateMediaSource_本地媒体源"

# 生成覆盖率报告
go test -v -race -coverprofile=debug/dao_coverage.out ./internal/dao/... -run "MediaSource"
go tool cover -html=debug/dao_coverage.out -o debug/dao_coverage.html
```

#### 2.2 执行 Service 层测试
```powershell
# 执行所有 Service 层测试
go test -v -race ./internal/service/... -run "MediaSource|FileOperation"

# 执行单个测试用例
go test -v -race ./internal/service/... -run "TestCreateMediaSource_正常流程"

# 生成覆盖率报告
go test -v -race -coverprofile=debug/service_coverage.out ./internal/service/... -run "MediaSource|FileOperation"
go tool cover -html=debug/service_coverage.out -o debug/service_coverage.html
```

#### 2.3 执行全部测试并生成覆盖率报告
```powershell
# 执行全部测试
go test -v -race -coverprofile=debug/coverage.out ./internal/...

# 生成 HTML 覆盖率报告
go tool cover -html=debug/coverage.html

# 查看覆盖率摘要
go tool cover -func=debug/coverage.out
```

#### 2.4 执行性能测试
```powershell
# 执行性能测试
go test -v -run=XXX -bench=. ./internal/service/... -benchmem

# 执行大量文件性能测试
go test -v -run TestFileListPerformance_大量文件 ./internal/service/...
```

---

## 三、测试覆盖率说明

### 1. 覆盖率目标

| 模块 | 目标覆盖率 | 说明 |
|:---|:---:|:---|
| DAO 层 | ≥ 80% | 数据访问层核心逻辑 |
| Service 层 | ≥ 75% | 业务逻辑层 |
| 文件操作 | ≥ 70% | 包含文件系统操作 |
| **总体** | **≥ 75%** | 综合覆盖率 |

### 2. 覆盖率统计命令

```powershell
# 查看总体覆盖率
go tool cover -func=debug/coverage.out | grep total

# 查看各文件覆盖率
go tool cover -func=debug/coverage.out

# 输出示例：
# internal/dao/media_source_dao.go:50% statements,80% functions
# internal/service/media_source_service.go:60% statements,75% functions
# internal/service/file_operation_service.go:70% statements,70% functions
```

### 3. 覆盖率报告解读

- **statements**：语句覆盖率（代码行执行比例）
- **functions**：函数覆盖率（函数调用比例）
- **branches**：分支覆盖率（条件分支执行比例）

---

## 四、测试环境配置

### 1. 数据库配置

#### 方式一：使用环境变量
```powershell
# PowerShell
$env:TEST_DB_URL="host=192.168.31.12 port=15432 user=joshua password=dwwzbuwioalvb dbname=easy_strm_test sslmode=disable"
```

#### 方式二：使用 .env.test 文件
```bash
# 文件路径：easy-strm/.env.test
TEST_DB_URL=host=192.168.31.12 port=15432 user=joshua password=dwwzbuwioalvb dbname=easy_strm_test sslmode=disable
```

### 2. 初始化测试数据库

```powershell
# 连接到 PostgreSQL
psql -h 192.168.31.12 -p 15432 -U joshua -d postgres

# 创建测试数据库
CREATE DATABASE easy_strm_test;

# 执行初始化脚本
\c easy_strm_test
\i migrations/migrate_mediamanager_test.sql
```

### 3. 清理测试数据

```powershell
# 清理所有测试数据
psql -h 192.168.31.12 -p 15432 -U joshua -d easy_strm_test -c "DELETE FROM t_media_source WHERE name LIKE '测试%';"
```

---

## 五、测试报告

### 1. 测试报告位置

- **HTML 覆盖率报告**：`debug/coverage.html`
- **测试日志**：`debug/test_results/test.log`
- **覆盖率数据**：`debug/coverage.out`

### 2. 测试报告内容

- 测试用例执行结果（通过/失败）
- 代码覆盖率统计
- 未覆盖代码高亮显示
- 执行时间统计

### 3. 持续集成

```yaml
# .github/workflows/test.yml 示例
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:12
        env:
          POSTGRES_DB: easy_strm_test
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
        ports:
          - 5432:5432
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: 1.20
      - name: Run tests
        env:
          TEST_DB_URL: "host=localhost port=5432 user=test password=test dbname=easy_strm_test sslmode=disable"
        run: |
          go test -v -race -coverprofile=coverage.out ./internal/...
          go tool cover -func=coverage.out
```

---

## 六、常见问题

### Q1: 测试数据库连接失败
**解决方案**：
1. 检查 `TEST_DB_URL` 环境变量是否正确
2. 确认数据库服务已启动
3. 验证网络连接和防火墙设置

### Q2: 文件操作权限不足
**解决方案**：
1. 以管理员身份运行测试
2. 检查临时目录权限
3. 确认防病毒软件未拦截

### Q3: 覆盖率报告未生成
**解决方案**：
1. 确认 `debug` 目录存在
2. 检查磁盘空间
3. 使用 `-coverprofile` 参数

### Q4: 115 云端测试失败
**解决方案**：
1. 确认有真实的 115 账号
2. 检查 Cookie 是否有效
3. 跳过云端测试：`go test -v -run "TestLocal.*"`

---

## 七、测试最佳实践

### 1. 测试命名规范
- 测试函数名：`Test<功能>_<场景>`
- 示例：`TestCreateMediaSource_本地媒体源`

### 2. 测试数据管理
- 使用 `TestMain` 初始化测试环境
- 测试后清理测试数据
- 避免测试数据污染

### 3. 断言规范
- 使用 `t.Fatalf` 标记致命错误
- 使用 `t.Errorf` 标记非致命错误
- 使用 `t.Logf` 记录调试信息

### 4. 并发测试
- 使用 `-race` 参数检测竞态条件
- 使用 `t.Parallel()` 并行执行测试

---

**最后更新**：2026-03-29  
**维护者**：质量保障专家
