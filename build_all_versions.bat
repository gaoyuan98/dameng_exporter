@echo off
setlocal enabledelayedexpansion

REM 设置变量
set PROGRAM_NAME=dameng_exporter

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

echo Building %PROGRAM_NAME% version: %VERSION%

REM 设置需要打包的配置文件
set CONFIG_FILES=dameng_exporter.toml custom_queries.metrics


REM 编译Windows 64位版本
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s -w" -o %PROGRAM_NAME%_windows_amd64.exe
if %errorlevel% neq 0 (
    echo Error compiling Windows 64-bit version
    timeout /t 3 /nobreak >nul
    exit /b 1
)
echo Compiled Windows 64-bit version successfully

REM 创建临时目录结构
set TEMP_DIR=%PROGRAM_NAME%_%VERSION%_windows_amd64
if exist %TEMP_DIR% rmdir /s /q %TEMP_DIR%
mkdir %TEMP_DIR%

REM 复制文件到临时目录
move %PROGRAM_NAME%_windows_amd64.exe %TEMP_DIR%\%PROGRAM_NAME%.exe
for %%f in (%CONFIG_FILES%) do (
    if exist %%f copy %%f %TEMP_DIR%\ >nul
)

REM 打包Windows版本为tar.gz
tar -czf %PROGRAM_NAME%_%VERSION%_windows_amd64.tar.gz %TEMP_DIR%
if %errorlevel% neq 0 (
    echo Error packaging Windows 64-bit version
    rmdir /s /q %TEMP_DIR%
    timeout /t 3 /nobreak >nul
    exit /b 1
)

REM 清理临时目录
rmdir /s /q %TEMP_DIR%

REM 编译Linux 64位版本
set GOOS=linux
set GOARCH=amd64
go build -ldflags "-s -w" -o %PROGRAM_NAME%_linux_amd64
if %errorlevel% neq 0 (
    echo Error compiling Linux 64-bit version
    timeout /t 3 /nobreak >nul
    exit /b 1
)
echo Compiled Linux 64-bit version successfully

REM 创建临时目录结构
set TEMP_DIR=%PROGRAM_NAME%_%VERSION%_linux_amd64
if exist %TEMP_DIR% rmdir /s /q %TEMP_DIR%
mkdir %TEMP_DIR%

REM 复制文件到临时目录
move %PROGRAM_NAME%_linux_amd64 %TEMP_DIR%\%PROGRAM_NAME%
for %%f in (%CONFIG_FILES%) do (
    if exist %%f copy %%f %TEMP_DIR%\ >nul
)

REM 打包Linux版本为tar.gz
tar -czf %PROGRAM_NAME%_%VERSION%_linux_amd64.tar.gz %TEMP_DIR%
if %errorlevel% neq 0 (
    echo Error packaging Linux 64-bit version
    rmdir /s /q %TEMP_DIR%
    timeout /t 3 /nobreak >nul
    exit /b 1
)

REM 清理临时目录
rmdir /s /q %TEMP_DIR%


REM 编译Linux ARM版本
set GOOS=linux
set GOARCH=arm64
set GOHOSTARCH=arm64
go build -ldflags "-s -w" -o %PROGRAM_NAME%_linux_arm64
if %errorlevel% neq 0 (
    echo Error compiling Linux ARM version
    timeout /t 3 /nobreak >nul
    exit /b 1
)
echo Compiled Linux ARM version successfully

REM 创建临时目录结构
set TEMP_DIR=%PROGRAM_NAME%_%VERSION%_linux_arm64
if exist %TEMP_DIR% rmdir /s /q %TEMP_DIR%
mkdir %TEMP_DIR%

REM 复制文件到临时目录
move %PROGRAM_NAME%_linux_arm64 %TEMP_DIR%\%PROGRAM_NAME%
for %%f in (%CONFIG_FILES%) do (
    if exist %%f copy %%f %TEMP_DIR%\ >nul
)

REM 打包Linux ARM版本为tar.gz
tar -czf %PROGRAM_NAME%_%VERSION%_linux_arm64.tar.gz %TEMP_DIR%
if %errorlevel% neq 0 (
    echo Error packaging Linux ARM version
    rmdir /s /q %TEMP_DIR%
    timeout /t 3 /nobreak >nul
    exit /b 1
)

REM 清理临时目录
rmdir /s /q %TEMP_DIR%

REM 编译darwin amd64版本
@REM SET GOOS=darwin
@REM SET GOARCH=amd64
@REM go build -ldflags "-s -w" -o %PROGRAM_NAME%_%VERSION%_darwin_amd64
@REM if %errorlevel% neq 0 (
@REM     echo Error compiling Darwin amd64 version
@REM     timeout /t 3 /nobreak >nul  REM 等待3秒钟
@REM     exit /b 1
@REM )
@REM echo Compiled Darwin amd64 version successfully
@REM REM 打包Darwin amd64版本为tar.gz，包括配置文件
@REM tar -czvf %PROGRAM_NAME%_%VERSION%_darwin_amd64.tar.gz %PROGRAM_NAME%_%VERSION%_darwin_amd64 %CONFIG_FILE%
@REM if %errorlevel% neq 0 (
@REM     echo Error packaging Darwin amd64 version
@REM     timeout /t 3 /nobreak >nul
@REM     exit /b 1
@REM )
@REM REM 清理编译生成的可执行文件
@REM del %PROGRAM_NAME%_%VERSION%_darwin_amd64


echo.
echo ======================================
echo All versions compiled successfully!
echo Version: %VERSION%
echo Package files include:
for %%f in (%CONFIG_FILES%) do (
    echo   - %%f
)
echo ======================================
echo.
exit /b 0
