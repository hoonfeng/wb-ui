@echo off
REM ============================================================
REM cgo_env.bat - set up wb-ui CGO build environment
REM Usage:
REM   cgo_env              - show env status
REM   cgo_env build        - go build ./...
REM   cgo_env test         - go test ./...
REM   cgo_env test -v      - go test -v ./...
REM   cgo_env test-all     - go test -count=1 (all pkgs except dev/suites/consistency)
REM   cgo_env run <target> - go run <target>
REM
REM Skia native library directory resolution order:
REM   1) SKIA_DLL_DIR if already set in the environment
REM   2) <goskia module dir>\skia\lib\windows_amd64, located via `go list -m`
REM
REM GOWORK is forced to "off" below. The parent F:\syproject\go.work has a
REM `use ./GWui` entry whose directory no longer holds a go.mod, so ANY `go`
REM command run from wb-ui under workspace mode dies with
REM   cannot load module ..\GWui listed in go.work file
REM With the workspace disabled, goskia resolves through go.mod (module cache)
REM instead of the local checkout -- `go list -m` above also needs this.
REM ============================================================

set CGO_ENABLED=1
set "GOWORK=off"

if not defined SKIA_DLL_DIR (
    for /f "delims=" %%d in ('go list -m -f "{{.Dir}}" github.com/hoonfeng/goskia 2^>nul') do set "SKIA_DLL_DIR=%%d\skia\lib\windows_amd64"
)
if not defined SKIA_DLL_DIR (
    echo [WARN] goskia module not found - cannot locate libSkiaSharp.dll
    echo        run: go mod download github.com/hoonfeng/goskia
    echo        or set SKIA_DLL_DIR to the directory holding libSkiaSharp.dll
    set "SKIA_DLL_DIR=."
)
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

REM ---------------------------------------------------------------------------
REM NOTE (round-22 fix): the build/test branches must test the exit code with
REM   `if errorlevel 1` (evaluated at RUN TIME). The previous form
REM   `if %ERRORLEVEL% EQU 0` sits INSIDE the parenthesized block, so cmd.exe
REM   expands %ERRORLEVEL% while parsing the whole block -> it freezes the value
REM   from before the block (0, from the `go version` line above), which made a
REM   FAILED `go build` still print [BUILD OK].
REM   Keep this file ASCII-only: a UTF-8 .bat read under code page 936 garbles
REM   the bytes, and stray brackets inside a block comment also break the
REM   parenthesized-block pairing (both reproduced on 2026-10-06).
REM ---------------------------------------------------------------------------
REM NOTE (round-22 fix #2): never put a failing `exit /b N` inside the
REM   parenthesized if/else-if dispatch block. Measured on this machine: the
REM   same `exit /b 9` propagates from a nested block in one shape and silently
REM   returns 0 in another (7-line scratch .bat reproductions e2/e3/e6), so the
REM   construct is not trustworthy. Dispatch is done with top-level
REM   `if ... goto :label` instead, and each handler keeps its exit code via a
REM   top-level, bracket-free `if errorlevel 1 goto :xxx_failed`.

if /i "%1"=="build" goto :do_build
if /i "%1"=="test"  goto :do_test
if /i "%1"=="run"   goto :do_run
if /i "%1"=="test-all" goto :do_test_all
if "%1"==""         goto :usage

echo [ERROR] Unknown subcommand: %1
echo Usage: cgo_env [build^|test^|test-all^|run^|<empty>]
exit /b 1

:do_build
echo ^>^> go build ./...
go build ./...
if errorlevel 1 goto :build_failed
echo [BUILD OK]
exit /b 0

:build_failed
echo [BUILD FAILED]
exit /b 1

:do_test
echo ^>^> go test %2 %3 %4 %5 ./...
go test %2 %3 %4 %5 ./...
if errorlevel 1 goto :test_failed
echo [ALL TESTS PASSED]
exit /b 0

:test_failed
echo [SOME TESTS FAILED]
exit /b 1

:do_test_all
REM Full-suite entry (Q6-B+D, 2026-10-07). Runs every package EXCEPT
REM dev/suites/consistency: that suite boots a real Edge and blocks for
REM minutes, which is exactly why a bare `go test ./...` never finishes here.
REM The package list is built with `go list` and filtered with findstr, then
REM handed to ONE go test invocation (per-package loops pay the cgo link cost
REM 96 times). Measured runtime on this machine: about 24 s.
setlocal enabledelayedexpansion
set "PKGS="
for /f "usebackq delims=" %%p in (`go list ./... ^| findstr /v /c:"dev/suites/consistency"`) do set "PKGS=!PKGS! %%p"
echo ^>^> go test -count=1 (all packages except dev/suites/consistency)
go test -count=1 !PKGS!
set "RC=!ERRORLEVEL!"
endlocal & set "RC=%RC%"
if not "%RC%"=="0" goto :test_all_failed
echo [TEST-ALL PASSED]
exit /b 0

:test_all_failed
echo [TEST-ALL: SOME TESTS FAILED]
exit /b 1

:do_run
shift
echo ^>^> go run %*
go run %*
if errorlevel 1 exit /b 1
exit /b 0

:usage
echo.
echo Commands:
echo   cgo_env build       - build all packages
echo   cgo_env test        - run all tests
echo   cgo_env test -v     - run all tests ^(verbose^)
echo   cgo_env test-all    - all pkgs except dev/suites/consistency ^(~24s^)
echo   cgo_env run main.go - run a program
echo.
exit /b 0
