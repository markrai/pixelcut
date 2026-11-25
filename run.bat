@echo off
setlocal
pushd "%~dp0" || exit /b 1
where go >nul 2>&1
if errorlevel 1 (
    echo Go was not found on PATH.
    goto :failed
)
echo Building PixelCut...
go build -o pixelcut-gui.exe ./cmd/pixelcut-gui
if errorlevel 1 goto :failed
echo Launching PixelCut...
pixelcut-gui.exe
if errorlevel 1 goto :failed
popd
exit /b 0
:failed
echo PixelCut could not build or run successfully.
pause
popd
exit /b 1
