@echo off
cd /d "%~dp0"

set PYTHON=.venv\Scripts\python.exe
set MODEL=data\jsut\dur_model.pth

if not exist "%PYTHON%" (
    echo error: Python venv not found at %PYTHON%
    exit /b 1
)
if not exist "%MODEL%" (
    echo error: model not found at %MODEL%
    exit /b 1
)

echo === UtauTTS Dev Server ===
echo.
echo Open http://127.0.0.1:8080
echo.

go run ./cmd/utautts-server --voice-dir sample --model %MODEL% --python %PYTHON% --tools tools
