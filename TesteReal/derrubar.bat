
@echo off
:: derrubar.bat
:: Encerra todos os processos go run iniciados pelo subir.bat
 
echo Encerrando todos os processos...
 
taskkill /F /IM go.exe /T >nul 2>&1
taskkill /F /FI "WINDOWTITLE eq Sensor Temperatura*" >nul 2>&1
taskkill /F /FI "WINDOWTITLE eq Sensor Umidade*" >nul 2>&1
taskkill /F /FI "WINDOWTITLE eq Atuador*" >nul 2>&1
taskkill /F /FI "WINDOWTITLE eq Cliente*" >nul 2>&1
 
echo Tudo encerrado.
pause