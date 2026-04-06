@echo off
REM ===================== MediaManager 模块测试执行脚本 =====================
REM 作者：质量保障专家
REM 日期：2026-03-29
REM 说明：执行 MediaManager 模块的单元测试并生成覆盖率报告

echo ========================================
echo MediaManager 模块测试执行脚本
echo ========================================
echo.

REM 设置测试数据库连接（使用环境变量或默认值）
if "%TEST_DB_URL%"=="" (
    echo [警告] 未设置 TEST_DB_URL 环境变量，使用默认测试数据库
    echo 默认数据库：192.168.31.12:15432/easy_strm_test
    echo.
)

REM 切换到项目目录
cd /d "%~dp0.."

REM 创建测试输出目录
if not exist "debug\test_results" mkdir debug\test_results

echo [步骤 1/4] 清理测试缓存...
go clean -testcache

echo.
echo [步骤 2/4] 执行 DAO 层测试...
go test -v -race -coverprofile=debug/test_results/dao_coverage.out ./internal/dao/... -run "MediaSource"
if errorlevel 1 (
    echo [错误] DAO 层测试失败
    goto :error
)

echo.
echo [步骤 3/4] 执行 Service 层测试...
go test -v -race -coverprofile=debug/test_results/service_coverage.out ./internal/service/... -run "MediaSource|FileOperation"
if errorlevel 1 (
    echo [错误] Service 层测试失败
    goto :error
)

echo.
echo [步骤 4/4] 生成覆盖率报告...

REM 合并覆盖率报告
echo mode: set > debug/test_results/coverage.out
type debug\test_results\dao_coverage.out | findstr /v "mode:" >> debug/test_results/coverage.out
type debug\test_results\service_coverage.out | findstr /v "mode:" >> debug/test_results/coverage.out

REM 生成 HTML 覆盖率报告
go tool cover -html=debug/test_results/coverage.out -o debug/test_results/coverage.html

REM 显示覆盖率摘要
echo.
echo ========================================
echo 测试覆盖率摘要
echo ========================================
go tool cover -func=debug/test_results/coverage.out | findstr "total"

echo.
echo ========================================
echo 测试执行完成
echo ========================================
echo 测试报告已保存到：debug\test_results\coverage.html
echo.

REM 打开覆盖率报告
start debug\test_results\coverage.html

goto :end

:error
echo.
echo ========================================
echo 测试执行失败，请检查错误日志
echo ========================================
exit /b 1

:end
exit /b 0
