@echo off
setlocal
set "BUILD_DIR="
pushd "%~dp0.." || exit /b 1

set "PROGRAM_NAME=dameng_exporter"
set "VERSION="
for /f "tokens=4" %%i in ('findstr /C:"const Version" dameng_exporter.go') do set "VERSION=%%~i"
if not defined VERSION (
    echo Error: Cannot find Version in dameng_exporter.go
    goto :fail
)

set "GIT_REVISION=unknown"
set "GIT_BRANCH=unknown"
for /f "delims=" %%i in ('git rev-parse HEAD 2^>nul') do set "GIT_REVISION=%%i"
for /f "delims=" %%i in ('git rev-parse --abbrev-ref HEAD 2^>nul') do set "GIT_BRANCH=%%i"
set "LDFLAGS=-s -w -X dameng_exporter/collector.GitRevision=%GIT_REVISION% -X dameng_exporter/collector.GitBranch=%GIT_BRANCH%"

REM Run the README helper for the host before cross-compiling.
set "GOOS="
set "GOARCH="
set "CGO_ENABLED=0"
go run -mod=readonly scripts\update_readme_downloads.go
if errorlevel 1 goto :fail

set "DIST_DIR=%CD%\dist"
if not exist "%DIST_DIR%" mkdir "%DIST_DIR%"
set "BUILD_DIR=%DIST_DIR%\.build-%RANDOM%-%RANDOM%"
if exist "%BUILD_DIR%" (
    echo Error: Build directory already exists
    set "BUILD_DIR="
    goto :fail
)
mkdir "%BUILD_DIR%"
if errorlevel 1 goto :fail

echo Building %PROGRAM_NAME% %VERSION% [%GIT_BRANCH% %GIT_REVISION%]
call :build windows amd64
if errorlevel 1 goto :fail
call :build linux amd64
if errorlevel 1 goto :fail
call :build linux arm64
if errorlevel 1 goto :fail
call :build darwin amd64
if errorlevel 1 goto :fail
call :build darwin arm64
if errorlevel 1 goto :fail

for %%f in ("%BUILD_DIR%\*.tar.gz") do (
    move /Y "%%~ff" "%DIST_DIR%\" >nul
    if errorlevel 1 goto :fail
)
rmdir /s /q "%BUILD_DIR%"
echo All 5 packages are ready in %DIST_DIR%
popd
exit /b 0

:build
set "GOOS=%~1"
set "GOARCH=%~2"
set "PACKAGE_NAME=%PROGRAM_NAME%_%VERSION%_%GOOS%_%GOARCH%"
set "PACKAGE_DIR=%BUILD_DIR%\%PACKAGE_NAME%"
set "BINARY_NAME=%PROGRAM_NAME%"
if "%GOOS%"=="windows" set "BINARY_NAME=%PROGRAM_NAME%.exe"
echo Building %GOOS%/%GOARCH%...
mkdir "%PACKAGE_DIR%"
if errorlevel 1 exit /b 1
go build -mod=readonly -trimpath -ldflags "%LDFLAGS%" -o "%PACKAGE_DIR%\%BINARY_NAME%" .
if errorlevel 1 exit /b 1
copy /Y docs\config\dameng_exporter.toml "%PACKAGE_DIR%\" >nul
if errorlevel 1 exit /b 1
copy /Y docs\config\custom_queries.metrics "%PACKAGE_DIR%\" >nul
if errorlevel 1 exit /b 1
copy /Y LICENSE "%PACKAGE_DIR%\" >nul
if errorlevel 1 exit /b 1
xcopy /E /I /Y docs\dashboards "%PACKAGE_DIR%\dashboards\" >nul
if errorlevel 1 exit /b 1
tar -czf "%BUILD_DIR%\%PACKAGE_NAME%.tar.gz" -C "%BUILD_DIR%" "%PACKAGE_NAME%"
exit /b %errorlevel%

:fail
if defined BUILD_DIR if exist "%BUILD_DIR%" rmdir /s /q "%BUILD_DIR%"
echo Build failed. No complete release was produced.
popd
exit /b 1
