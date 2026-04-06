# Complete API Test Script - Fixed for nested data structure
$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "MediaManager Module Round 5 API Test" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$testResults = @()

# Test 1: Login
Write-Host "[Test 1] Login" -ForegroundColor Yellow
try {
    $json = '{"name":"admin","password":"21232f297a57a5a743894a0e4a801fc3"}'
    $response = Invoke-RestMethod -Uri "http://localhost:8082/login" -Method POST -Body $json -ContentType "application/json; charset=utf-8"
    $token = $response.token
    $headers = @{Authorization = "Bearer $token"}
    Write-Host "  [PASS] Login successful" -ForegroundColor Green
    $testResults += "[PASS] Login"
} catch {
    Write-Host "  [FAIL] Login failed: $($_.Exception.Message)" -ForegroundColor Red
    $testResults += "[FAIL] Login"
    exit 1
}
Write-Host ""

# Test 2: Create Media Source
Write-Host "[Test 2] Create Media Source" -ForegroundColor Yellow
try {
    $json = '{"name":"TestUpdateSource_Round5","source_type":"local","path":"c:\\Users\\a3875\\Documents\\code\\easy-strm\\debug\\test","priority":10,"enabled":true}'
    $response = Invoke-WebRequest -Uri "http://localhost:8082/media/sources" -Method POST -Headers $headers -Body $json -ContentType "application/json; charset=utf-8" -UseBasicParsing
    $parsed = $response.Content | ConvertFrom-Json

    # Handle nested data structure
    $sourceId = $parsed.data.data.id
    Write-Host "  [PASS] Created, ID: $sourceId" -ForegroundColor Green
    $testResults += "[PASS] Create Media Source"
} catch {
    Write-Host "  [FAIL] Create failed: $($_.Exception.Message)" -ForegroundColor Red
    $testResults += "[FAIL] Create Media Source"
    exit 1
}
Write-Host ""

# Test 3: Update Media Source (Bug 1 Fix)
Write-Host "[Test 3] Update Media Source (Bug 1 Fix)" -ForegroundColor Yellow
try {
    $json = '{"name":"UpdatedSourceName","priority":20}'
    $response = Invoke-WebRequest -Uri "http://localhost:8082/media/sources/$sourceId" -Method PUT -Headers $headers -Body $json -ContentType "application/json; charset=utf-8" -UseBasicParsing
    $parsed = $response.Content | ConvertFrom-Json

    # Handle nested data structure
    $updatedName = $parsed.data.data.name
    $updatedPriority = $parsed.data.data.priority

    if ($updatedName -eq "UpdatedSourceName" -and $updatedPriority -eq 20) {
        Write-Host "  [PASS] Updated correctly - Name: $updatedName, Priority: $updatedPriority" -ForegroundColor Green
        $testResults += "[PASS] Update Media Source"
    } else {
        Write-Host "  [FAIL] Update not effective - Name: $updatedName, Priority: $updatedPriority" -ForegroundColor Red
        $testResults += "[FAIL] Update Media Source"
    }
} catch {
    Write-Host "  [FAIL] Update failed: $($_.Exception.Message)" -ForegroundColor Red
    $testResults += "[FAIL] Update Media Source"
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
    $body = @{
        source_id = $sourceId
        file_id = "rename_test.mp4"
        new_name = "renamed_file.mp4"
    }
    $json = $body | ConvertTo-Json -Compress
    $response = Invoke-WebRequest -Uri "http://localhost:8082/media/files/rename" -Method POST -Headers $headers -Body $json -ContentType "application/json; charset=utf-8" -UseBasicParsing

    $files = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File
    $renamedFile = $files | Where-Object { $_.Name -like "renamed_file*" }

    if ($renamedFile.Name -eq "renamed_file.mp4") {
        Write-Host "  [PASS] Renamed correctly: $($renamedFile.Name)" -ForegroundColor Green
        $testResults += "[PASS] Rename File"
    } elseif ($renamedFile.Name -eq "renamed_file.mp4.mp4") {
        Write-Host "  [FAIL] Extension duplicated: $($renamedFile.Name)" -ForegroundColor Red
        $testResults += "[FAIL] Rename File (extension duplicated)"
    } else {
        Write-Host "  [FAIL] Unexpected filename: $($renamedFile.Name)" -ForegroundColor Red
        $testResults += "[FAIL] Rename File"
    }
} catch {
    Write-Host "  [FAIL] Rename failed: $($_.Exception.Message)" -ForegroundColor Red
    $testResults += "[FAIL] Rename File"
}
Write-Host ""

# Test 6: Delete File
Write-Host "[Test 5] Delete File" -ForegroundColor Yellow
try {
    $body = @{
        source_id = $sourceId
        file_ids = @("renamed_file.mp4")
    }
    $json = $body | ConvertTo-Json -Compress
    $response = Invoke-WebRequest -Uri "http://localhost:8082/media/files/delete" -Method POST -Headers $headers -Body $json -ContentType "application/json; charset=utf-8" -UseBasicParsing

    $remainingFiles = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File -ErrorAction SilentlyContinue
    if ($remainingFiles.Count -eq 0) {
        Write-Host "  [PASS] File deleted" -ForegroundColor Green
        $testResults += "[PASS] Delete File"
    } else {
        Write-Host "  [FAIL] Still have $($remainingFiles.Count) files" -ForegroundColor Red
        $testResults += "[FAIL] Delete File"
    }
} catch {
    Write-Host "  [FAIL] Delete failed: $($_.Exception.Message)" -ForegroundColor Red
    $testResults += "[FAIL] Delete File"
}
Write-Host ""

# Test 7: Delete Media Source
Write-Host "[Test 6] Delete Media Source" -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8082/media/sources/$sourceId" -Method DELETE -Headers $headers -UseBasicParsing
    Write-Host "  [PASS] Deleted" -ForegroundColor Green
    $testResults += "[PASS] Delete Media Source"
} catch {
    Write-Host "  [FAIL] Delete failed: $($_.Exception.Message)" -ForegroundColor Red
    $testResults += "[FAIL] Delete Media Source"
}
Write-Host ""

# Summary
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Test Results Summary" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
$testResults | ForEach-Object { Write-Host "  $_" }
Write-Host ""

$passCount = ($testResults | Where-Object { $_ -like "*PASS*" }).Count
$failCount = ($testResults | Where-Object { $_ -like "*FAIL*" }).Count
Write-Host "Total: $passCount passed, $failCount failed" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Red" })
Write-Host ""

if ($failCount -eq 0) {
    Write-Host "Status: Pass" -ForegroundColor Green
} else {
    Write-Host "Status: Fail" -ForegroundColor Red
}
