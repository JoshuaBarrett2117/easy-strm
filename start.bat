@echo off
setlocal

set "ROOT=%~dp0"
set "BACKEND_PORT=8082"
set "FRONTEND_PORT=3001"

echo [1/4] Releasing backend port %BACKEND_PORT%...
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%BACKEND_PORT% .*LISTENING"') do taskkill /F /PID %%P >nul 2>&1

echo [2/4] Releasing frontend port %FRONTEND_PORT%...
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%FRONTEND_PORT% .*LISTENING"') do taskkill /F /PID %%P >nul 2>&1

echo [3/4] Starting backend on http://localhost:%BACKEND_PORT% ...
start "easy-strm backend" /D "%ROOT%easy-strm" cmd /d /c "%ROOT%debug\easy-strm-library-fix.exe > backend-dev.log 2>&1"

echo [4/4] Starting frontend on http://localhost:%FRONTEND_PORT% ...
start "easy-strm frontend" /D "%ROOT%easy-strm-front" cmd /d /c "npm run dev -- --host 0.0.0.0 --port %FRONTEND_PORT% --strictPort > frontend-dev.log 2>&1"

echo Start commands have been issued. Check easy-strm\backend-dev.log and easy-strm-front\frontend-dev.log if needed.
endlocal
