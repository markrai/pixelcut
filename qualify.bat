@echo off
setlocal
pushd "%~dp0" || exit /b 1
echo Checking FFmpeg transformation equivalence...
go test -count=1 -v ./internal/media -run "^TestTransformPixelEquivalence$"
if errorlevel 1 goto :failed
echo Checking native mpv preview against export...
set "PIXELCUT_QUALIFY_PREVIEW=1"
go test -count=1 -timeout 20m -v ./cmd/pixelcut-gui -run "^TestNativePreviewEquivalence$"
if errorlevel 1 goto :failed
echo Automated qualification passed. Embedded real-footage acceptance is still required.
pause
popd
exit /b 0
:failed
echo Qualification failed. Review the output above.
pause
popd
exit /b 1
