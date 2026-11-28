@echo off
setlocal
pushd "%~dp0" || exit /b 1
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0scripts\ci.ps1" %*
set "CI_EXIT=%ERRORLEVEL%"
popd
exit /b %CI_EXIT%
