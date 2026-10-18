@echo off
chcp 65001>nul
rem 作成日: 2026-10-18。開発用Goコマンドの書込みをリポジトリ内へ限定する。
setlocal
cd /d "%~dp0"
set "GOPATH=%~dp0..\.cache\gopath"
set "GOCACHE=%~dp0..\.cache\go-build"
set "GOMODCACHE=%~dp0..\.cache\modules"
set "GOTMPDIR=%~dp0..\.cache\tmp"
set "GOTELEMETRYDIR=%~dp0..\.cache\telemetry"
set "TEMP=%GOTMPDIR%"
set "TMP=%GOTMPDIR%"
set "GOTOOLCHAIN=go1.26.4"
set "CGO_ENABLED=0"
if "%~1"=="test" if "%~2"=="-race" set "CGO_ENABLED=1"
if not exist "%GOTMPDIR%" mkdir "%GOTMPDIR%"
if errorlevel 1 exit /b 1
go %*
exit /b %errorlevel%
