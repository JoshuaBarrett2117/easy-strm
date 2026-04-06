# Test Script
$testResults = @()

# Test 1: Login
Write-Host "[Test 1] Login" -ForegroundColor Yellow
try {
    $body = @{
        name = "admin"
        password = "21232f297a57a5a743894a0e4a801fc3"
    } | ConvertTo-Json

    $loginResp = Invoke-RestMethod -Uri "http://localhost:8082/login" -Method POST -Body $body -ContentType "application/json"
    $token = $loginResp.token
    $headers = @{Authorization = "Bearer $token"}
    Write-Host "  [PASS] Login successful" -ForegroundColor Green
    $testResults += "[PASS] Login"
} catch {
    Write-Host "  [FAIL] Login failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Login"
    exit 1
}

# Test 2: Create Media Source
Write-Host "[Test 2] Create Media Source" -ForegroundColor Yellow
try {
    $createBody = @{
        name = "TestUpdateSource"
        source_type = "local"
        path = "c:\Users\a3875\Documents\code\easy-strm\debug\test"
        priority = 10
        enabled = $true
    } | ConvertTo-Json

    $createResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources" -Method POST -Headers $headers -Body $createBody -ContentType "application/json"
    $sourceId = $createResp.data.id
    Write-Host "  [PASS] Created, ID: $sourceId" -ForegroundColor Green
    $testResults += "[PASS] Create Media Source"
} catch {
    Write-Host "  [FAIL] Create failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Create Media Source"
}

# Test 3: Get Media Source List
Write-Host "[Test 3] Get Media Source List" -ForegroundColor Yellow
try {
    $listResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources" -Method GET -Headers $headers
    Write-Host "  [PASS] Count: $($listResp.data.Count)" -ForegroundColor Green
    $testResults += "[PASS] Get Media Source List"
} catch {
    Write-Host "  [FAIL] Get failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Get Media Source List"
}

# Test 4: Update Media Source (Bug 1 Fix)
Write-Host "[Test 4] Update Media Source (Bug 1 Fix)" -ForegroundColor Yellow
try {
    $updateBody = @{
        name = "UpdatedSourceName"
        priority = 20
    } | ConvertTo-Json

    $updateResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources/$sourceId" -Method PUT -Headers $headers -Body $updateBody -ContentType "application/json"

    if ($updateResp.data.name -eq "UpdatedSourceName" -and $updateResp.data.priority -eq 20) {
        Write-Host "  [PASS] Updated correctly - Name: $($updateResp.data.name), Priority: $($updateResp.data.priority)" -ForegroundColor Green
        $testResults += "[PASS] Update Media Source"
    } else {
        Write-Host "  [FAIL] Update not effective" -ForegroundColor Red
        $testResults += "[FAIL] Update Media Source"
    }
} catch {
    Write-Host "  [FAIL] Update failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Update Media Source"
}

# Test 5: Create Test File
Write-Host "[Test Preparation] Create test file" -ForegroundColor Yellow
Set-Content -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test\rename_test.mp4" -Value "test content"
Write-Host "  Test file created" -ForegroundColor Green

# Test 6: Get File List
Write-Host "[Test 5] Get File List" -ForegroundColor Yellow
try {
    $fileListResp = Invoke-RestMethod -Uri "http://localhost:8082/media/files?source_id=$sourceId" -Method GET -Headers $headers
    Write-Host "  [PASS] Count: $($fileListResp.data.Count)" -ForegroundColor Green
    $testResults += "[PASS] Get File List"
} catch {
    Write-Host "  [FAIL] Get failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Get File List"
}

# Test 7: Rename File (Bug 2 Fix)
Write-Host "[Test 6] Rename File (Bug 2 Fix)" -ForegroundColor Yellow
try {
    $renameBody = @{
        source_id = $sourceId
        file_id = "rename_test.mp4"
        new_name = "renamed_file.mp4"
    } | ConvertTo-Json

    $renameResp = Invoke-RestMethod -Uri "http://localhost:8082/media/files/rename" -Method POST -Headers $headers -Body $renameBody -ContentType "application/json"

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
    Write-Host "  [FAIL] Rename failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Rename File"
}

# Test 8: Delete File
Write-Host "[Test 7] Delete File" -ForegroundColor Yellow
try {
    $deleteBody = @{
        source_id = $sourceId
        file_ids = @("renamed_file.mp4")
    } | ConvertTo-Json

    $deleteResp = Invoke-RestMethod -Uri "http://localhost:8082/media/files/delete" -Method POST -Headers $headers -Body $deleteBody -ContentType "application/json"

    $remainingFiles = Get-ChildItem -Path "c:\Users\a3875\Documents\code\easy-strm\debug\test" -File -ErrorAction SilentlyContinue
    if ($remainingFiles.Count -eq 0) {
        Write-Host "  [PASS] File deleted" -ForegroundColor Green
        $testResults += "[PASS] Delete File"
    } else {
        Write-Host "  [FAIL] Still have files" -ForegroundColor Red
        $testResults += "[FAIL] Delete File"
    }
} catch {
    Write-Host "  [FAIL] Delete failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Delete File"
}

# Test 9: Delete Media Source
Write-Host "[Test 8] Delete Media Source" -ForegroundColor Yellow
try {
    $deleteSourceResp = Invoke-RestMethod -Uri "http://localhost:8082/media/sources/$sourceId" -Method DELETE -Headers $headers
    Write-Host "  [PASS] Deleted" -ForegroundColor Green
    $testResults += "[PASS] Delete Media Source"
} catch {
    Write-Host "  [FAIL] Delete failed: $_" -ForegroundColor Red
    $testResults += "[FAIL] Delete Media Source"
}

# Summary
Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host "Test Results Summary" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
$testResults | ForEach-Object { Write-Host "  $_" }

$passCount = ($testResults | Where-Object { $_ -like "*PASS*" }).Count
$failCount = ($testResults | Where-Object { $_ -like "*FAIL*" }).Count
Write-Host "`nTotal: $passCount passed, $failCount failed" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Red" })

if ($failCount -eq 0) {
    Write-Host "`nStatus: Pass" -ForegroundColor Green
} else {
    Write-Host "`nStatus: Fail" -ForegroundColor Red
}
