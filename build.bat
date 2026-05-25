@echo off
cd /d "%~dp0"

echo === Building Go executables ===
go build -o utautts-core.exe ./cmd/utautts-core
go build -o utautts-server.exe ./cmd/utautts-server
go build -o utautts.exe ./cmd/utautts

echo === Building Python engine (PyInstaller) ===
call build_engine.bat

echo === Creating release directories ===
set REL=release

rmdir /s /q "%REL%\UtauTTS" 2>nul
mkdir "%REL%\UtauTTS\core"
mkdir "%REL%\UtauTTS\models"
mkdir "%REL%\UtauTTS\voice"
copy utautts.exe "%REL%\UtauTTS\"
copy engine.exe "%REL%\UtauTTS\core\"
copy utautts-core.exe "%REL%\UtauTTS\core\"

rmdir /s /q "%REL%\UtauTTS-Server" 2>nul
mkdir "%REL%\UtauTTS-Server\core"
mkdir "%REL%\UtauTTS-Server\models"
mkdir "%REL%\UtauTTS-Server\voice"
copy utautts-server.exe "%REL%\UtauTTS-Server\"
copy engine.exe "%REL%\UtauTTS-Server\core\"
copy utautts-core.exe "%REL%\UtauTTS-Server\core\"

echo === Done ===
dir /s /b "%REL%"
