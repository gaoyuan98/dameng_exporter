@echo off
setlocal enabledelayedexpansion

echo ======================================
echo Docker Hub Push Script for dameng_exporter
echo Target Repository: gaoyuan98/dameng_exporter
echo ======================================
echo.

REM 从 Go 源码中提取版本号
for /f "tokens=4" %%i in ('findstr /C:"const Version" dameng_exporter.go') do (
    set VERSION=%%i
    REM 移除引号
    set VERSION=!VERSION:"=!
)

if "%VERSION%"=="" (
    echo Error: Cannot find version in dameng_exporter.go
    exit /b 1
)

echo Version: %VERSION%
echo.

REM 检查本地镜像是否存在
echo Checking local images...
docker images dameng_exporter:%VERSION%-linux-amd64 >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: Local image dameng_exporter:%VERSION%-linux-amd64 not found
    echo Please build images first using docker-build-linux-multiarch.bat
    exit /b 1
)

docker images dameng_exporter:%VERSION%-linux-arm64 >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: Local image dameng_exporter:%VERSION%-linux-arm64 not found
    echo Please build images first using docker-build-linux-multiarch.bat
    exit /b 1
)

echo Local images found!
echo.

REM 提醒用户检查登录状态
echo IMPORTANT: Please ensure you are logged in to Docker Hub.
echo If not logged in, run: docker login
echo.

REM 创建 Docker Hub 标签
echo Creating tags for Docker Hub...

REM AMD64 标签
docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:latest-linux-amd64
docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:%VERSION%
docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:latest

REM ARM64 标签
docker tag dameng_exporter:%VERSION%-linux-arm64 gaoyuan98/dameng_exporter:%VERSION%-linux-arm64
docker tag dameng_exporter:%VERSION%-linux-arm64 gaoyuan98/dameng_exporter:latest-linux-arm64

echo Tags created successfully!
echo.

REM 推送镜像
echo ======================================
echo Starting push to Docker Hub...
echo ======================================
echo.

REM 推送 AMD64 版本
echo [1/6] Pushing gaoyuan98/dameng_exporter:%VERSION%-linux-amd64...
docker push gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
if %errorlevel% neq 0 goto push_error

echo [2/6] Pushing gaoyuan98/dameng_exporter:latest-linux-amd64...
docker push gaoyuan98/dameng_exporter:latest-linux-amd64
if %errorlevel% neq 0 goto push_error

REM 推送 ARM64 版本
echo [3/6] Pushing gaoyuan98/dameng_exporter:%VERSION%-linux-arm64...
docker push gaoyuan98/dameng_exporter:%VERSION%-linux-arm64
if %errorlevel% neq 0 goto push_error

echo [4/6] Pushing gaoyuan98/dameng_exporter:latest-linux-arm64...
docker push gaoyuan98/dameng_exporter:latest-linux-arm64
if %errorlevel% neq 0 goto push_error

REM 推送通用标签（默认指向 AMD64）
echo [5/6] Pushing gaoyuan98/dameng_exporter:%VERSION% (default/amd64)...
docker push gaoyuan98/dameng_exporter:%VERSION%
if %errorlevel% neq 0 goto push_error

echo [6/6] Pushing gaoyuan98/dameng_exporter:latest (default/amd64)...
docker push gaoyuan98/dameng_exporter:latest
if %errorlevel% neq 0 goto push_error

echo.
echo ======================================
echo SUCCESS: All images pushed to Docker Hub!
echo ======================================
echo.
echo View your images at:
echo   https://hub.docker.com/r/gaoyuan98/dameng_exporter/tags
echo.
echo Available tags:
echo   gaoyuan98/dameng_exporter:%VERSION% (default, points to AMD64)
echo   gaoyuan98/dameng_exporter:latest (default, points to AMD64)
echo   gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
echo   gaoyuan98/dameng_exporter:latest-linux-amd64
echo   gaoyuan98/dameng_exporter:%VERSION%-linux-arm64
echo   gaoyuan98/dameng_exporter:latest-linux-arm64
echo.
echo Pull commands:
echo   docker pull gaoyuan98/dameng_exporter:latest
echo   docker pull gaoyuan98/dameng_exporter:%VERSION%
echo   docker pull gaoyuan98/dameng_exporter:latest-linux-arm64
echo ======================================
echo.
exit /b 0

:push_error
echo.
echo ======================================
echo ERROR: Failed to push images to Docker Hub
echo ======================================
echo.
echo Possible causes:
echo   1. Not logged in to Docker Hub (run: docker login)
echo   2. No permission to push to gaoyuan98/dameng_exporter
echo   3. Network connection issues
echo.
exit /b 1