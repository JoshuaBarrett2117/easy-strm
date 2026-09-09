@echo off
setlocal

set "BACKEND_PORT=8082"
set "FRONTEND_PORT=3001"

echo Stopping backend processes listening on port %BACKEND_PORT%...
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%BACKEND_PORT% .*LISTENING"') do taskkill /F /PID %%P >nul 2>&1

echo Stopping frontend processes listening on port %FRONTEND_PORT%...
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%FRONTEND_PORT% .*LISTENING"') do taskkill /F /PID %%P >nul 2>&1

echo Backend and frontend stop commands have been issued.
endlocal
