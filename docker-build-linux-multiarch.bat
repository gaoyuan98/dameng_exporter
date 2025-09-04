@echo off
setlocal enabledelayedexpansion

REM 获取 Git 信息
for /f "tokens=*" %%i in ('git rev-parse HEAD 2^>nul') do set GIT_REVISION=%%i
if "%GIT_REVISION%"=="" set GIT_REVISION=unknown

for /f "tokens=*" %%i in ('git rev-parse --abbrev-ref HEAD 2^>nul') do set GIT_BRANCH=%%i
if "%GIT_BRANCH%"=="" set GIT_BRANCH=unknown

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

echo ======================================
echo Building Docker images for Linux platforms
echo   Version: %VERSION%
echo   Git Revision: %GIT_REVISION%
echo   Git Branch: %GIT_BRANCH%
echo ======================================
echo.

REM 构建 Linux AMD64 (x86_64) 版本
echo [1/2] Building Linux AMD64 (x86_64) image...
echo --------------------------------------
docker build ^
    --build-arg GIT_REVISION=%GIT_REVISION% ^
    --build-arg GIT_BRANCH=%GIT_BRANCH% ^
    --build-arg GOARCH=amd64 ^
    -t dameng_exporter:%VERSION%-linux-amd64 ^
    -t dameng_exporter:latest-linux-amd64 ^
    .

if %errorlevel% neq 0 (
    echo Error building Linux AMD64 Docker image
    exit /b 1
)
echo Linux AMD64 image built successfully!
echo.

REM 构建 Linux ARM64 版本
echo [2/2] Building Linux ARM64 image...
echo --------------------------------------
docker build ^
    --build-arg GIT_REVISION=%GIT_REVISION% ^
    --build-arg GIT_BRANCH=%GIT_BRANCH% ^
    --build-arg GOARCH=arm64 ^
    -t dameng_exporter:%VERSION%-linux-arm64 ^
    -t dameng_exporter:latest-linux-arm64 ^
    .

if %errorlevel% neq 0 (
    echo Error building Linux ARM64 Docker image
    exit /b 1
)
echo Linux ARM64 image built successfully!
echo.

echo ======================================
echo All Linux Docker images built successfully!
echo.
echo Images created:
echo   - dameng_exporter:%VERSION%-linux-amd64
echo   - dameng_exporter:latest-linux-amd64
echo   - dameng_exporter:%VERSION%-linux-arm64  
echo   - dameng_exporter:latest-linux-arm64
echo ======================================
echo.

REM 询问是否推送到 Docker Hub
set /p PUSH_TO_HUB="Do you want to push images to Docker Hub? (y/n): "
if /i "%PUSH_TO_HUB%"=="y" (
    echo.
    echo Tagging images for Docker Hub (gaoyuan98/dameng_exporter)...
    
    REM 为 Docker Hub 创建标签 - AMD64
    docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
    docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:latest-linux-amd64
    
    REM 为 Docker Hub 创建标签 - ARM64
    docker tag dameng_exporter:%VERSION%-linux-arm64 gaoyuan98/dameng_exporter:%VERSION%-linux-arm64
    docker tag dameng_exporter:%VERSION%-linux-arm64 gaoyuan98/dameng_exporter:latest-linux-arm64
    
    echo.
    echo Pushing images to Docker Hub...
    echo Please ensure you are logged in to Docker Hub (docker login)
    echo.
    
    REM 推送 AMD64 版本
    echo [1/4] Pushing gaoyuan98/dameng_exporter:%VERSION%-linux-amd64...
    docker push gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
    if %errorlevel% neq 0 (
        echo Error pushing AMD64 version image
        echo Please check your Docker Hub login status
        exit /b 1
    )
    
    echo [2/4] Pushing gaoyuan98/dameng_exporter:latest-linux-amd64...
    docker push gaoyuan98/dameng_exporter:latest-linux-amd64
    if %errorlevel% neq 0 (
        echo Error pushing AMD64 latest image
        exit /b 1
    )
    
    REM 推送 ARM64 版本
    echo [3/4] Pushing gaoyuan98/dameng_exporter:%VERSION%-linux-arm64...
    docker push gaoyuan98/dameng_exporter:%VERSION%-linux-arm64
    if %errorlevel% neq 0 (
        echo Error pushing ARM64 version image
        exit /b 1
    )
    
    echo [4/4] Pushing gaoyuan98/dameng_exporter:latest-linux-arm64...
    docker push gaoyuan98/dameng_exporter:latest-linux-arm64
    if %errorlevel% neq 0 (
        echo Error pushing ARM64 latest image
        exit /b 1
    )
    
    echo.
    echo ======================================
    echo All images pushed to Docker Hub successfully!
    echo.
    echo View at: https://hub.docker.com/r/gaoyuan98/dameng_exporter/tags
    echo.
    echo Pull commands:
    echo   docker pull gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
    echo   docker pull gaoyuan98/dameng_exporter:%VERSION%-linux-arm64
    echo   docker pull gaoyuan98/dameng_exporter:latest-linux-amd64
    echo   docker pull gaoyuan98/dameng_exporter:latest-linux-arm64
    echo ======================================
) else (
    echo.
    echo To run the containers locally:
    echo   docker run -d -p 9200:9200 dameng_exporter:latest-linux-amd64
    echo   docker run -d -p 9200:9200 dameng_exporter:latest-linux-arm64
    echo.
    echo To push manually later:
    echo   docker tag dameng_exporter:%VERSION%-linux-amd64 gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
    echo   docker push gaoyuan98/dameng_exporter:%VERSION%-linux-amd64
)
echo.