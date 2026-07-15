@echo off
REM ============================================================
REM cgo_env.bat - set up wb-ui CGO build environment
REM Usage:
REM   cgo_env              - show env status
REM   cgo_env build        - go build ./...
REM   cgo_env test         - go test ./...
REM   cgo_env test -v      - go test -v ./...
REM   cgo_env run <target> - go run <target>
REM ============================================================

set CGO_ENABLED=1
set SKIA_DLL_DIR=F:\syproject\goskia\bin
set PATH=%SKIA_DLL_DIR%;%PATH%

echo ========================================
echo    wb-ui CGO Environment
echo ========================================
echo   CGO_ENABLED = %CGO_ENABLED%
where gcc >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    for /f "delims=" %%i in ('where gcc') do echo   CC          = %%i
) else (
    echo   CC          = [not found]
)
echo   SKIA_DLL    = %SKIA_DLL_DIR%\libSkiaSharp.dll
if exist "%SKIA_DLL_DIR%\libSkiaSharp.dll" (
    echo   DLL status  = [FOUND]
) else (
    echo   DLL status  = [NOT FOUND]
)
for /f "delims=" %%i in ('go version') do echo   Go version  = %%i
echo ========================================

if /i "%1"=="build" (
    echo ^>^> go build ./...
    go build ./...
    if %ERRORLEVEL% EQU 0 ( echo [BUILD OK] ) else ( echo [BUILD FAILED] )
) else if /i "%1"=="test" (
    echo ^>^> go test %2 %3 %4 %5 ./...
    go test %2 %3 %4 %5 ./...
    if %ERRORLEVEL% EQU 0 ( echo [ALL TESTS PASSED] ) else ( echo [SOME TESTS FAILED] )
) else if /i "%1"=="run" (
    shift
    echo ^>^> go run %*
    go run %*
) else if "%1"=="" (
    echo.
    echo Commands:
    echo   cgo_env build       - build all packages
    echo   cgo_env test        - run all tests
    echo   cgo_env test -v     - run all tests ^(verbose^)
    echo   cgo_env run main.go - run a program
    echo.
) else (
    echo [ERROR] Unknown subcommand: %1
    echo Usage: cgo_env [build^|test^|run^|<empty>]
    exit /b 1
)
