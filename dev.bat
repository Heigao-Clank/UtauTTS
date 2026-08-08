@echo off
cd /d "%~dp0"

echo === UtauTTS Dev Server ===
echo.
echo Open http://127.0.0.1:8080
echo.

go run ./cmd/utautts-server --voice-dir sample
