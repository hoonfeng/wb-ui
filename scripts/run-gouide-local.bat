@echo off
REM 启动 PairCode IDE 桌面端（wb-ui 本地加载 + 请求拦截模式）。
REM
REM 用法：
REM   scripts\run-gouide-local.bat                 :: 自动发现后端并拉起（默认端口 9098）
REM   scripts\run-gouide-local.bat -attach -port 9099
REM   scripts\run-gouide-local.bat -http           :: 回退整站 HTTP 加载
REM
REM 环境：CGO=1 + goskia/bin 进 PATH；go.work 本机破损（GWui 目录为空），
REM       一律 GOWORK=off 按 go.mod 解析 goskia。
setlocal

set ROOT=%~dp0..
for %%I in ("%ROOT%") do set ROOT=%%~fI
set SIBLING=%ROOT%\..

set CGO_ENABLED=1
set GOWORK=off
set PATH=%SIBLING%\goskia\bin;%PATH%

cd /d "%ROOT%"

if not exist "_temp\gouide.exe" (
  echo [gouide] 首次运行，构建 _temp\gouide.exe ...
  go build -o _temp\gouide.exe .\cmd\gouide
  if errorlevel 1 (
    echo [gouide] 构建失败
    exit /b 1
  )
)

_temp\gouide.exe %*
endlocal
