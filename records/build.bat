@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo Building records for Windows and macOS
echo ========================================

cd /d "%~dp0"
if not exist dist mkdir dist

:: Windows x86 (amd64)
echo.
echo [1/2] Building for Windows x86 (amd64)...
set GOOS=windows
set GOARCH=amd64
go build -trimpath -o dist\records-windows-amd64.exe .
if %errorlevel% neq 0 (
    echo Build failed for Windows amd64
    exit /b 1
)
echo OK: dist\records-windows-amd64.exe

:: macOS ARM64 (Apple Silicon)
echo.
echo [2/2] Building for macOS ARM64 (Apple Silicon)...
set GOOS=darwin
set GOARCH=arm64
go build -trimpath -o dist\records-darwin-arm64 .
if %errorlevel% neq 0 (
    echo Build failed for macOS arm64
    exit /b 1
)
echo OK: dist\records-darwin-arm64

echo.
echo ========================================
echo Build completed successfully!
echo   Windows:  dist\records-windows-amd64.exe
echo   macOS:    dist\records-darwin-arm64
echo ========================================
exit /b 0
