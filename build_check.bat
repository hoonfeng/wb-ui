@echo off
set CGO_ENABLED=1
go build .\dev\window_test\main.go > build_log.txt 2>&1
echo EXIT CODE: %ERRORLEVEL%
