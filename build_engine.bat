@echo off
cd /d "%~dp0"

echo === Installing PyInstaller ===
uv pip install pyinstaller

echo === Building engine.exe ===
pyinstaller --onefile --name engine --distpath . tools\engine.py

echo === Copying to release dirs ===
if exist "release\UtauTTS\core\" copy /y engine.exe "release\UtauTTS\core\" >nul
if exist "release\UtauTTS-Server\core\" copy /y engine.exe "release\UtauTTS-Server\core\" >nul

echo === Cleaning up ===
rmdir /s /q build 2>nul
del /q engine.spec 2>nul

echo === Done: engine.exe ===
