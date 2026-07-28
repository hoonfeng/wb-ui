@echo off
cd /d F:\syproject\wb-ui
set CGO_ENABLED=1
go build ./app/ > F:\syproject\wb-ui\_build_out.txt 2>&1
echo RET=%ERRORLEVEL%
