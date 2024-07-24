@echo off

REM 编译Windows 64位版本
set GOOS=windows
set GOARCH=amd64
go build -o myprogram.exe
if %errorlevel% neq 0 (
    echo Error compiling Windows 64-bit version
    timeout /t 3 /nobreak >nul  REM 等待3秒钟
    exit /b 1
)
echo Compiled Windows 64-bit version successfully


REM 编译Linux 64位版本
set GOOS=linux
set GOARCH=amd64
go build -o myprogram_linux
if %errorlevel% neq 0 (
    echo Error compiling Linux 64-bit version
    timeout /t 3 /nobreak >nul  REM 等待3秒钟
    exit /b 1
)
echo Compiled Linux 64-bit version successfully

REM 编译Linux ARM版本

SET GOOS=linux
SET GOARCH=arm64
SET GOHOSTARCH=arm64
go build -o myprogram_arm
if %errorlevel% neq 0 (
    echo Error compiling Linux ARM version
    timeout /t 3 /nobreak >nul  REM 等待3秒钟
    exit /b 1
)
echo Compiled Linux ARM version successfully
echo All versions compiled successfully
exit /b 0
