@echo on
cd /d F:\syproject\wb-ui
set "PATH=C:\Program Files\Go\bin;%PATH%"
set CGO_ENABLED=1
where go
go version
go build ./app/  2> F:\syproject\wb-ui\_build_err.log
 
echo EXIT: %ERRORLEVEL%
type F:\syproject\wb-ui\_build_err.log
