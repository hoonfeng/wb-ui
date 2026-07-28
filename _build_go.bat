@echo off
cd /d F:\syproject\wb-ui
set CGO_ENABLED=1
set GOFLAGS=
go clean -cache >nul 2>&1
go build -o NUL .\app\ 2> F:\syproject\wb-ui\_err3.txt
echo RC=%ERRORLEVEL%
if exist F:\syproject\wb-ui\_err3.txt type F:\syproject\wb-ui\_err3.txt
