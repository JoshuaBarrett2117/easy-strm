# Debug Login Test
$ErrorActionPreference = "Continue"

Write-Host "Testing Login API..." -ForegroundColor Yellow

# Create the request body
$body = @{
    name = "admin"
    password = "21232f297a57a5a743894a0e4a801fc3"
} | ConvertTo-Json -Compress

Write-Host "Request Body: $body" -ForegroundColor Gray

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8082/login" -Method POST -Body $body -ContentType "application/json" -UseBasicParsing
    Write-Host "Status Code: $($response.StatusCode)" -ForegroundColor Green
    Write-Host "Response: $($response.Content)" -ForegroundColor Green
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        Write-Host "Status Code: $($_.Exception.Response.StatusCode.value__)" -ForegroundColor Red
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $reader.BaseStream.Position = 0
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response Body: $responseBody" -ForegroundColor Red
    }
}

# Try with different password encoding
Write-Host "`nTrying with explicit JSON string..." -ForegroundColor Yellow
$jsonString = '{"name":"admin","password":"21232f297a57a5a743894a0e4a801fc3"}'
Write-Host "JSON String: $jsonString" -ForegroundColor Gray

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8082/login" -Method POST -Body $jsonString -ContentType "application/json; charset=utf-8" -UseBasicParsing
    Write-Host "Status Code: $($response.StatusCode)" -ForegroundColor Green
    Write-Host "Response: $($response.Content)" -ForegroundColor Green
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        Write-Host "Status Code: $($_.Exception.Response.StatusCode.value__)" -ForegroundColor Red
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $reader.BaseStream.Position = 0
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response Body: $responseBody" -ForegroundColor Red
    }
}
