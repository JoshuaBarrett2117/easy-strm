# MediaManager 模块第五轮 API 测试脚本
# 测试目标：验证 Bug 1（更新媒体源）和 Bug 2（重命名文件扩展名重复）的修复

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "MediaManager 模块第五轮 API 测试" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 测试结果记录
$testResults = @()

# ========== 1. 登录测试 ==========
Write-Host "[测试 1] 登录获取 Token" -ForegroundColor Yellow
try {
    $md5Password = "21232f297a57a5a743894a0e4a801fc3"
    $body = @{name="admin"; password=$md5Password} | ConvertTo-Json -Compress
    $loginResp = Invoke-RestMethod -Uri "http://localhost:8082/login" -Method POST -Body $body -ContentType "application/json"
    $token = $loginResp.token
    $headers = @{Authorization="Bearer $token"}
    Write-Host "  ✓ 登录成功" -ForegroundColor Green
    Write-Host "  Token: $($token.Substring(0,20))..." -ForegroundColor Gray
    $testResults += "✓ 登录测试: Pass"
} catch {
    Write-Host "  ✗ 登录失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 登录测试: Fail"
    exit 1
}
Write-Host ""

# ========== 2. 创建媒体源测试 ==========
Write-Host "[测试 2] 创建媒体源" -ForegroundColor Yellow
try {
    $createBody = @{
        name="TestUpdateSource"
        source_type="local"
        path="c:\Users\a3875\Documents\code\easy-strm\debug\test"
        priority=10
        enabled=$true
    } | ConvertTo-Json -Compress

    $createResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources" -Method POST -Headers $headers -Body $createBody -ContentType "application/json"
    $sourceId = $createResp.data.id
    Write-Host "  ✓ 创建成功" -ForegroundColor Green
    Write-Host "  媒体源 ID: $sourceId" -ForegroundColor Gray
    Write-Host "  名称: $($createResp.data.name)" -ForegroundColor Gray
    Write-Host "  优先级: $($createResp.data.priority)" -ForegroundColor Gray
    $testResults += "✓ 创建媒体源: Pass"
} catch {
    Write-Host "  ✗ 创建失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 创建媒体源: Fail"
}
Write-Host ""

# ========== 3. 获取媒体源列表测试 ==========
Write-Host "[测试 3] 获取媒体源列表" -ForegroundColor Yellow
try {
    $listResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources" -Method GET -Headers $headers
    Write-Host "  ✓ 获取成功" -ForegroundColor Green
    Write-Host "  媒体源数量: $($listResp.data.Count)" -ForegroundColor Gray
    $testResults += "✓ 获取媒体源列表: Pass"
} catch {
    Write-Host "  ✗ 获取失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 获取媒体源列表: Fail"
}
Write-Host ""

# ========== 4. 更新媒体源测试（验证 Bug 1 修复）==========
Write-Host "[测试 4] 更新媒体源（验证 Bug 1 修复）" -ForegroundColor Yellow
try {
    $updateBody = @{
        name="UpdatedSourceName"
        priority=20
    } | ConvertTo-Json -Compress

    $updateResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources/$sourceId" -Method PUT -Headers $headers -Body $updateBody -ContentType "application/json"
    Write-Host "  ✓ 更新成功" -ForegroundColor Green
    Write-Host "  更新后名称: $($updateResp.data.name)" -ForegroundColor Gray
    Write-Host "  更新后优先级: $($updateResp.data.priority)" -ForegroundColor Gray

    # 验证更新是否生效
    if ($updateResp.data.name -eq "UpdatedSourceName" -and $updateResp.data.priority -eq 20) {
        Write-Host "  ✓ 验证通过：name 和 priority 已正确更新" -ForegroundColor Green
        $testResults += "✓ 更新媒体源: Pass"
    } else {
        Write-Host "  ✗ 验证失败：更新未生效" -ForegroundColor Red
        $testResults += "✗ 更新媒体源: Fail（更新未生效）"
    }
} catch {
    Write-Host "  ✗ 更新失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 更新媒体源: Fail"
}
Write-Host ""

# ========== 5. 创建测试文件 ==========
Write-Host "[测试准备] 创建测试文件" -ForegroundColor Yellow
Set-Content -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test\rename_test.mp4" -Value "test content"
Write-Host "  ✓ 测试文件已创建: rename_test.mp4" -ForegroundColor Green
Write-Host ""

# ========== 6. 获取文件列表测试 ==========
Write-Host "[测试 5] 获取文件列表" -ForegroundColor Yellow
try {
    $fileListResp = Invoke-RestMethod -Uri "http://localhost:8082/media/files?source_id=$sourceId" -Method GET -Headers $headers
    Write-Host "  ✓ 获取成功" -ForegroundColor Green
    Write-Host "  文件数量: $($fileListResp.data.Count)" -ForegroundColor Gray
    $testResults += "✓ 获取文件列表: Pass"
} catch {
    Write-Host "  ✗ 获取失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 获取文件列表: Fail"
}
Write-Host ""

# ========== 7. 重命名文件测试（验证 Bug 2 修复）==========
Write-Host "[测试 6] 重命名文件（验证 Bug 2 修复）" -ForegroundColor Yellow
try {
    $renameBody = @{
        source_id=$sourceId
        file_id="rename_test.mp4"
        new_name="renamed_file.mp4"
    } | ConvertTo-Json -Compress

    $renameResp = Invoke-RestMethod -Uri "http://localhost:8082/media/files/rename" -Method POST -Headers $headers -Body $renameBody -ContentType "application/json"
    Write-Host "  ✓ 重命名成功" -ForegroundColor Green

    # 验证文件是否正确重命名（不应该有 .mp4.mp4）
    $files = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File
    $renamedFile = $files | Where-Object { $_.Name -like "renamed_file*" }

    if ($renamedFile.Name -eq "renamed_file.mp4") {
        Write-Host "  ✓ 验证通过：文件名为 renamed_file.mp4（扩展名未重复）" -ForegroundColor Green
        $testResults += "✓ 重命名文件: Pass"
    } elseif ($renamedFile.Name -eq "renamed_file.mp4.mp4") {
        Write-Host "  ✗ 验证失败：文件名为 renamed_file.mp4.mp4（扩展名重复）" -ForegroundColor Red
        $testResults += "✗ 重命名文件: Fail（扩展名重复）"
    } else {
        Write-Host "  ✗ 验证失败：未找到正确的重命名文件，实际文件名: $($renamedFile.Name)" -ForegroundColor Red
        $testResults += "✗ 重命名文件: Fail"
    }
} catch {
    Write-Host "  ✗ 重命名失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 重命名文件: Fail"
}
Write-Host ""

# ========== 8. 删除文件测试 ==========
Write-Host "[测试 7] 删除文件" -ForegroundColor Yellow
try {
    $deleteBody = @{
        source_id=$sourceId
        file_ids=@("renamed_file.mp4")
    } | ConvertTo-Json -Compress

    $deleteResp = Invoke-RestMethod -Uri "http://localhost:8082/media/files/delete" -Method POST -Headers $headers -Body $deleteBody -ContentType "application/json"
    Write-Host "  ✓ 删除成功" -ForegroundColor Green

    # 验证文件是否被删除
    $remainingFiles = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File
    if ($remainingFiles.Count -eq 0) {
        Write-Host "  ✓ 验证通过：文件已被删除" -ForegroundColor Green
        $testResults += "✓ 删除文件: Pass"
    } else {
        Write-Host "  ✗ 验证失败：仍有 $($remainingFiles.Count) 个文件" -ForegroundColor Red
        $testResults += "✗ 删除文件: Fail"
    }
} catch {
    Write-Host "  ✗ 删除失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 删除文件: Fail"
}
Write-Host ""

# ========== 9. 删除媒体源测试 ==========
Write-Host "[测试 8] 删除媒体源" -ForegroundColor Yellow
try {
    $deleteSourceResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources/$sourceId" -Method DELETE -Headers $headers
    Write-Host "  ✓ 删除成功" -ForegroundColor Green
    $testResults += "✓ 删除媒体源: Pass"
} catch {
    Write-Host "  ✗ 删除失败" -ForegroundColor Red
    Write-Host "  错误: $_" -ForegroundColor Red
    $testResults += "✗ 删除媒体源: Fail"
}
Write-Host ""

# ========== 测试结果汇总 ==========
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "测试结果汇总" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
$testResults | ForEach-Object { Write-Host "  $_" }
Write-Host ""

$passCount = ($testResults | Where-Object { $_ -like "*Pass*" }).Count
$failCount = ($testResults | Where-Object { $_ -like "*Fail*" }).Count
Write-Host "总计: $passCount 通过, $failCount 失败" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Red" })
Write-Host ""

if ($failCount -eq 0) {
    Write-Host "Status: Pass" -ForegroundColor Green
} else {
    Write-Host "Status: Fail" -ForegroundColor Red
}
