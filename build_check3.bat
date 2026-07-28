@echo on
cd /d F:\syproject\wb-ui
set CGO_ENABLED=1
go vet ./app/ 2>&1
echo VET_EXIT: %ERRORLEVEL%
