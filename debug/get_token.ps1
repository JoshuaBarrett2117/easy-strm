# 测试登录接口并获取Token
$loginUrl = "http://localhost:8082/login"
$body = @{
    username = "admin"
    password = "admin"
} | ConvertTo-Json

Write-Host "测试登录接口..." -ForegroundColor Yellow
Write-Host "URL: $loginUrl"
Write-Host "Body: $body"

try {
    $response = Invoke-RestMethod -Uri $loginUrl -Method POST -Body $body -ContentType "application/json; charset=utf-8"
    Write-Host "`n响应:" -ForegroundColor Cyan
    $response | ConvertTo-Json -Depth 10

    if ($response.state -eq $true) {
        $token = $response.data.token
        Write-Host "`n✅ 登录成功!" -ForegroundColor Green
        Write-Host "Token: $token" -ForegroundColor Cyan

        # 保存Token到文件
        $token | Out-File -FilePath "debug\token.txt" -Encoding UTF8
        Write-Host "Token已保存到 debug\token.txt" -ForegroundColor Green
    } else {
        Write-Host "`n❌ 登录失败: $($response.error)" -ForegroundColor Red
    }
} catch {
    Write-Host "`n❌ 请求异常: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "详细错误: $($_.ErrorDetails.Message)" -ForegroundColor Red
}
