@echo off
REM ============================================================
REM PairCode IDE 桌面版 —— 一体化启动脚本（单入口 / 单命令）
REM
REM 源码入口：wb-ui\cmd\gouide\        （Go 程序，main 包）
REM 构建产物：wb-ui\bin\pairgui.exe    （唯一可执行文件，双击即用）
REM
REM 它自动完成：发现 gou-ide 后端（pair.exe/companion.exe）→ 拉起后端
REM 子进程（WEB_PORT 分配到本程序端口，绝不动 9090）→ 加载前端 →
REM 创建原生窗口。关闭窗口时一并结束后端（-keep-alive 可保留）。
REM
REM 用法：
REM   scripts\gui.bat                     :: 构建（首次）+ 启动窗口
REM   scripts\gui.bat -port 9098          :: 指定端口
REM   scripts\gui.bat -attach -port 9099  :: 只连接已运行的后端
REM   scripts\gui.bat -headless -frames 100 -out x.png   :: 无窗口自检
REM
REM 环境：CGO=1 + goskia/bin 进 PATH；本机 go.work 破损（GWui 为空），
REM      一律 GOWORK=off 按 go.mod 解析 goskia。
REM ============================================================
setlocal

set ROOT=%~dp0..
for %%I in ("%ROOT%") do set ROOT=%%~fI
set SIBLING=%ROOT%\..

set CGO_ENABLED=1
set GOWORK=off
set PATH=%SIBLING%\goskia\bin;%PATH%

cd /d "%ROOT%"

if not exist "bin" mkdir bin
if not exist "bin\pairgui.exe" (
  echo [pairgui] 首次运行，构建 bin\pairgui.exe ...
  go build -o bin\pairgui.exe .\cmd\gouide
  if errorlevel 1 (
    echo [pairgui] 构建失败
    exit /b 1
  )
)

bin\pairgui.exe %*
endlocal
