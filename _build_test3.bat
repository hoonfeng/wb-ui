@echo on
cd /d F:\syproject\wb-ui
set CGO_ENABLED=1
go build -v .\dev\window_test\main.go 2> F:\syproject\wb-ui\_build_err2.log
echo EXIT: %ERRORLEVEL%
type F:\syproject\wb-ui\_build_err2.log
