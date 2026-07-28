@echo on
cd /d F:\syproject\wb-ui
set CGO_ENABLED=1
go build .\dev\window_test\main.go 2>&1
echo EXIT: %ERRORLEVEL%
