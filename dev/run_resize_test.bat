@echo off
set CGO_ENABLED=1
set SKIA_DLL_DIR=F:\syproject\goskia\bin
set PATH=F:\syproject\goskia\bin;%PATH%
echo === STARTING WINDOW TEST ===
echo Maximize the window, observe the purple sidebar, then close the window.
echo Output will be saved to resize_dump.txt
window_test2.exe --html F:\syproject\wb-ui\dev\pixel_probe\test_html\step15_float.html -w 800 -h 600 > resize_dump.txt 2>&1
echo === DONE ===
pause
