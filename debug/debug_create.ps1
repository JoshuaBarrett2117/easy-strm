# Debug Create Media Source
$ErrorActionPreference = "Stop"

Write-Host "Testing Create Media Source API..." -ForegroundColor Yellow

# Login first
$json = '{"name":"admin","password":"21232f297a57a5a743894a0e4a801fc3"}'
$loginResp = Invoke-RestMethod -Uri "http://localhost:8082/login" -Method POST -Body $json -ContentType "application/json; charset=utf-8"
$token = $loginResp.token
$headers = @{Authorization = "Bearer $token"}
Write-Host "Login successful, token: $($token.Substring(0,20))..." -ForegroundColor Green

# Create Media Source
Write-Host "`nCreating media source..." -ForegroundColor Yellow
$createJson = '{"name":"TestDebugSource","source_type":"local","path":"c:\\Users\\a3875\\Documents\\code\\easy-strm\\debug\\test","priority":10,"enabled":true}'

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8082/media/sources" -Method POST -Headers $headers -Body $createJson -ContentType "application/json; charset=utf-8" -UseBasicParsing
    Write-Host "Status Code: $($response.StatusCode)" -ForegroundColor Green
    Write-Host "Response Content:" -ForegroundColor Cyan
    Write-Host $response.Content -ForegroundColor White

    # Parse the response
    $parsed = $response.Content | ConvertFrom-Json
    Write-Host "`nParsed Response:" -ForegroundColor Cyan
    Write-Host "  message: $($parsed.message)" -ForegroundColor Gray
    Write-Host "  data.id: $($parsed.data.id)" -ForegroundColor Gray
    Write-Host "  data.name: $($parsed.data.name)" -ForegroundColor Gray
    Write-Host "  data.source_type: $($parsed.data.source_type)" -ForegroundColor Gray
    Write-Host "  data.path: $($parsed.data.path)" -ForegroundColor Gray
    Write-Host "  data.priority: $($parsed.data.priority)" -ForegroundColor Gray
    Write-Host "  data.enabled: $($parsed.data.enabled)" -ForegroundColor Gray

    if ($parsed.data.id) {
        Write-Host "`n[SUCCESS] ID is present: $($parsed.data.id)" -ForegroundColor Green
    } else {
        Write-Host "`n[ERROR] ID is missing or null!" -ForegroundColor Red
    }
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $reader.BaseStream.Position = 0
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response Body: $responseBody" -ForegroundColor Red
    }
}
