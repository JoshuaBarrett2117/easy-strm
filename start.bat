@echo off
setlocal

set "ROOT=%~dp0"
set "BACKEND_PORT=8082"
set "FRONTEND_PORT=3001"

echo [1/5] Building backend from current source...
if not exist "%ROOT%debug" mkdir "%ROOT%debug"
pushd "%ROOT%easy-strm"
go build -o "%ROOT%debug\easy-strm-dev-next.exe" .
if errorlevel 1 (
    popd
    echo Backend build failed. Existing services have been kept running.
    exit /b 1
)
popd

echo [2/5] Releasing backend port %BACKEND_PORT%...
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%BACKEND_PORT% .*LISTENING"') do taskkill /F /PID %%P >nul 2>&1

move /Y "%ROOT%debug\easy-strm-dev-next.exe" "%ROOT%debug\easy-strm-dev.exe" >nul
if errorlevel 1 (
    echo Failed to install the newly built backend. Check whether the previous process has exited.
    exit /b 1
)

echo [3/5] Releasing frontend port %FRONTEND_PORT%...
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%FRONTEND_PORT% .*LISTENING"') do taskkill /F /PID %%P >nul 2>&1

echo [4/5] Starting backend on http://localhost:%BACKEND_PORT% ...
start "easy-strm backend" /D "%ROOT%easy-strm" cmd /d /c ""%ROOT%debug\easy-strm-dev.exe" > backend-dev.log 2>&1"

echo [5/5] Starting frontend on http://localhost:%FRONTEND_PORT% ...
start "easy-strm frontend" /D "%ROOT%easy-strm-front" cmd /d /c "npm run dev -- --host 0.0.0.0 --port %FRONTEND_PORT% --strictPort > frontend-dev.log 2>&1"

echo Start commands have been issued. Check easy-strm\backend-dev.log and easy-strm-front\frontend-dev.log if needed.
endlocal
