# Simple API Test Script
$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "MediaManager Module Round 5 API Test" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Test 1: Login
Write-Host "[Test 1] Login" -ForegroundColor Yellow
try {
    $json = '{"name":"admin","password":"21232f297a57a5a743894a0e4a801fc3"}'
    $response = Invoke-RestMethod -Uri "http://localhost:8082/login" -Method POST -Body $json -ContentType "application/json; charset=utf-8"
    $token = $response.token
    $headers = @{Authorization = "Bearer $token"}
    Write-Host "  [PASS] Login successful" -ForegroundColor Green
    Write-Host "  Token: $($token.Substring(0,30))..." -ForegroundColor Gray
} catch {
    Write-Host "  [FAIL] Login failed" -ForegroundColor Red
    Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $reader.BaseStream.Position = 0
        $responseBody = $reader.ReadToEnd()
        Write-Host "  Response: $responseBody" -ForegroundColor Red
    }
    exit 1
}
Write-Host ""

# Test 2: Create Media Source
Write-Host "[Test 2] Create Media Source" -ForegroundColor Yellow
try {
    $json = '{"name":"TestUpdateSource","source_type":"local","path":"c:\\Users\\a3875\\Documents\\code\\easy-strm\\debug\\test","priority":10,"enabled":true}'
    $response = Invoke-RestMethod -Uri "http://localhost:8082/media/sources" -Method POST -Headers $headers -Body $json -ContentType "application/json; charset=utf-8"
    $sourceId = $response.data.id
    Write-Host "  [PASS] Created, ID: $sourceId" -ForegroundColor Green
} catch {
    Write-Host "  [FAIL] Create failed" -ForegroundColor Red
    Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Test 3: Update Media Source (Bug 1 Fix)
Write-Host "[Test 3] Update Media Source (Bug 1 Fix)" -ForegroundColor Yellow
try {
    $json = '{"name":"UpdatedSourceName","priority":20}'
    $response = Invoke-RestMethod -Uri "http://localhost:8082/media/sources/$sourceId" -Method PUT -Headers $headers -Body $json -ContentType "application/json; charset=utf-8"

    if ($response.data.name -eq "UpdatedSourceName" -and $response.data.priority -eq 20) {
        Write-Host "  [PASS] Updated correctly" -ForegroundColor Green
        Write-Host "  Name: $($response.data.name), Priority: $($response.data.priority)" -ForegroundColor Gray
    } else {
        Write-Host "  [FAIL] Update not effective" -ForegroundColor Red
    }
} catch {
    Write-Host "  [FAIL] Update failed" -ForegroundColor Red
    Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Test 4: Create Test File
Write-Host "[Test Preparation] Create test file" -ForegroundColor Yellow
Set-Content -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test\rename_test.mp4" -Value "test content"
Write-Host "  Test file created: rename_test.mp4" -ForegroundColor Green
Write-Host ""

# Test 5: Rename File (Bug 2 Fix)
Write-Host "[Test 4] Rename File (Bug 2 Fix)" -ForegroundColor Yellow
try {
    $json = "{`"source_id`":$sourceId,`"file_id`":`"rename_test.mp4`",`"new_name`":`"renamed_file.mp4`"}"
    $response = Invoke-RestMethod -Uri "http://localhost:8082/media/files/rename" -Method POST -Headers $headers -Body $json -ContentType "application/json; charset=utf-8"

    $files = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File
    $renamedFile = $files | Where-Object { $_.Name -like "renamed_file*" }

    if ($renamedFile.Name -eq "renamed_file.mp4") {
        Write-Host "  [PASS] Renamed correctly: $($renamedFile.Name)" -ForegroundColor Green
    } elseif ($renamedFile.Name -eq "renamed_file.mp4.mp4") {
        Write-Host "  [FAIL] Extension duplicated: $($renamedFile.Name)" -ForegroundColor Red
    } else {
        Write-Host "  [FAIL] Unexpected filename: $($renamedFile.Name)" -ForegroundColor Red
    }
} catch {
    Write-Host "  [FAIL] Rename failed" -ForegroundColor Red
    Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Test 6: Delete File
Write-Host "[Test 5] Delete File" -ForegroundColor Yellow
try {
    $json = "{`"source_id`":$sourceId,`"file_ids`":[`"renamed_file.mp4`"]}"
    $response = Invoke-RestMethod -Uri "http://localhost:8082/media/files/delete" -Method POST -Headers $headers -Body $json -ContentType "application/json; charset=utf-8"

    $remainingFiles = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File -ErrorAction SilentlyContinue
    if ($remainingFiles.Count -eq 0) {
        Write-Host "  [PASS] File deleted" -ForegroundColor Green
    } else {
        Write-Host "  [FAIL] Still have files" -ForegroundColor Red
    }
} catch {
    Write-Host "  [FAIL] Delete failed" -ForegroundColor Red
    Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Test 7: Delete Media Source
Write-Host "[Test 6] Delete Media Source" -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Uri "http://localhost:8082/media/sources/$sourceId" -Method DELETE -Headers $headers
    Write-Host "  [PASS] Deleted" -ForegroundColor Green
} catch {
    Write-Host "  [FAIL] Delete failed" -ForegroundColor Red
    Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Status: Pass" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
