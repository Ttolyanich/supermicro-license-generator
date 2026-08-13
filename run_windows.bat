@echo off
title Supermicro License Generator & SUM Activator (Windows Standalone)
cls
echo =========================================================================
echo  Supermicro License Generator & SUM Activator 2.15 (Windows)
echo =========================================================================
echo.

rem The app opens the browser itself once the server is up, so we do not
rem launch it here (that would open a second, empty tab).
if exist "supermicro-license-generator.exe" (
    echo [OK] Starting local server via supermicro-license-generator.exe...
    supermicro-license-generator.exe
    goto :end
)

where go >nul 2>nul
if %errorlevel% equ 0 (
    echo [OK] Go compiler detected. Launching via go run main.go...
    go run main.go
    goto :end
)

echo [!] NOTICE: Go compiler or pre-compiled supermicro-license-generator.exe not found.
echo [!] Please build supermicro-license-generator.exe using Go or run via Docker:
echo     docker compose up -d --build
echo.
pause

:end
