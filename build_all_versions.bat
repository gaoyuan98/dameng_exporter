@echo off


REM 设置变量
set PROGRAM_NAME=dameng_exporter
set VERSION=v1.0.0
set CONFIG_FILE=dameng_exporter.config


REM 编译Windows 64位版本
set GOOS=windows
set GOARCH=amd64
go build -o %PROGRAM_NAME%_%VERSION%_windows_amd64.exe
if %errorlevel% neq 0 (
    echo Error compiling Windows 64-bit version
    timeout /t 3 /nobreak >nul  REM 等待3秒钟
    exit /b 1
)
echo Compiled Windows 64-bit version successfully

REM 打包Windows版本为tar.gz，包括配置文件
tar -czvf %PROGRAM_NAME%_%VERSION%_windows_amd64.tar.gz %PROGRAM_NAME%_%VERSION%_windows_amd64.exe %CONFIG_FILE%
if %errorlevel% neq 0 (
    echo Error packaging Windows 64-bit version
    timeout /t 3 /nobreak >nul
    exit /b 1
)
REM 清理编译生成的可执行文件
del %PROGRAM_NAME%_%VERSION%_windows_amd64.exe

REM 编译Linux 64位版本
set GOOS=linux
set GOARCH=amd64
go build -o %PROGRAM_NAME%_%VERSION%_linux_amd64
if %errorlevel% neq 0 (
    echo Error compiling Linux 64-bit version
    timeout /t 3 /nobreak >nul  REM 等待3秒钟
    exit /b 1
)
echo Compiled Linux 64-bit version successfully
REM 打包Linux版本为tar.gz，包括配置文件
tar -czvf %PROGRAM_NAME%_%VERSION%_linux_amd64.tar.gz %PROGRAM_NAME%_%VERSION%_linux_amd64 %CONFIG_FILE%
if %errorlevel% neq 0 (
    echo Error packaging Linux 64-bit version
    timeout /t 3 /nobreak >nul
    exit /b 1
)

REM 清理编译生成的可执行文件
del %PROGRAM_NAME%_%VERSION%_linux_amd64


REM 编译Linux ARM版本

SET GOOS=linux
SET GOARCH=arm64
SET GOHOSTARCH=arm64
go build -o %PROGRAM_NAME%_%VERSION%_linux_arm64
if %errorlevel% neq 0 (
    echo Error compiling Linux ARM version
    timeout /t 3 /nobreak >nul  REM 等待3秒钟
    exit /b 1
)
echo Compiled Linux ARM version successfully
REM 打包Linux ARM版本为tar.gz，包括配置文件
tar -czvf %PROGRAM_NAME%_%VERSION%_linux_arm64.tar.gz %PROGRAM_NAME%_%VERSION%_linux_arm64 %CONFIG_FILE%
if %errorlevel% neq 0 (
    echo Error packaging Linux ARM version
    timeout /t 3 /nobreak >nul
    exit /b 1
)

REM 清理编译生成的可执行文件
del %PROGRAM_NAME%_%VERSION%_linux_arm64


echo All versions compiled successfully
exit /b 0
