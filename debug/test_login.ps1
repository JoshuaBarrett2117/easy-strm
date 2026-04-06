# 测试登录接口
$loginUrl = "http://localhost:8082/login"

# 测试不同的密码
$passwords = @("admin", "admin123", "123456", "password")

foreach ($pwd in $passwords) {
    Write-Host "`n测试密码: $pwd" -ForegroundColor Yellow

    $body = @{
        username = "admin"
        password = $pwd
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri $loginUrl -Method POST -Body $body -ContentType "application/json"

        if ($response.state -eq $true) {
            Write-Host "✅ 登录成功!" -ForegroundColor Green
            Write-Host "Token: $($response.data.token)" -ForegroundColor Cyan
            break
        } else {
            Write-Host "❌ 登录失败: $($response.error)" -ForegroundColor Red
        }
    } catch {
        Write-Host "❌ 请求异常: $($_.Exception.Message)" -ForegroundColor Red
    }
}
