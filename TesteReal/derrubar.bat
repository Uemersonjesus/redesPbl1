@echo off
echo Encerrando componentes...


taskkill /F /IM sensor.exe /T >nul 2>&1
taskkill /F /IM atuador.exe /T >nul 2>&1
taskkill /F /IM cliente.exe /T >nul 2>&1

echo Tudo encerrado.
pause