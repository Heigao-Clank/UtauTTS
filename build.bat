@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0tools\build-release.ps1"
exit /b %errorlevel%
