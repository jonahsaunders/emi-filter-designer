@echo off
rem Builds EMIFilterDesigner.exe - requires Go 1.22+ (https://go.dev/dl/)
cd /d "%~dp0"
if not exist dist mkdir dist
set CGO_ENABLED=0
go build -trimpath -ldflags "-H windowsgui -s -w" -o dist\EMIFilterDesigner.exe .
if errorlevel 1 (pause & exit /b 1)
echo Built dist\EMIFilterDesigner.exe
pause
