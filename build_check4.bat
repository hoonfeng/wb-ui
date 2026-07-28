@echo on
cd /d F:\syproject\wb-ui
set CGO_ENABLED=1
go build -gcflags="-e" ./app/ 2>&1
echo EXIT: %ERRORLEVEL%
