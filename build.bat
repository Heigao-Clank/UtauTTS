@echo off
setlocal
cd /d "%~dp0"
set "GOCACHE=%CD%\.tmp-go-cache"

echo === Testing ===
go test ./...
if errorlevel 1 exit /b 1

echo === Building ===
if not exist "release\UtauTTS" mkdir "release\UtauTTS"
go build -trimpath -o "release\UtauTTS\utautts.exe" ./cmd/utautts
if errorlevel 1 exit /b 1
go build -trimpath -o "release\UtauTTS\utautts-server.exe" ./cmd/utautts-server
if errorlevel 1 exit /b 1
go build -trimpath -o "release\UtauTTS\oto-inspect.exe" ./cmd/oto-inspect
if errorlevel 1 exit /b 1
go build -trimpath -o "release\UtauTTS\prosody-dataset.exe" ./cmd/prosody-dataset
if errorlevel 1 exit /b 1
go build -trimpath -o "release\UtauTTS\prosody-train.exe" ./cmd/prosody-train
if errorlevel 1 exit /b 1
copy /y README.md "release\UtauTTS\README.md" >nul
copy /y THIRD_PARTY_NOTICES.txt "release\UtauTTS\THIRD_PARTY_NOTICES.txt" >nul

echo === Done ===
dir /b "release\UtauTTS"
