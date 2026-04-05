@echo off
:: subir.bat
:: Uso: subir.bat <IP> <Sensores> <Atuadores> <Clientes>
:: Exemplo: subir.bat 192.168.1.10 3 3 2
 
set IP=%1
set SENSORES=%2
set ATUADORES=%3
set CLIENTES=%4
 
if "%IP%"=="" (
    echo Uso: subir.bat ^<IP^> ^<Sensores^> ^<Atuadores^> ^<Clientes^>
    echo Exemplo: subir.bat 192.168.1.10 3 3 2
    pause
    exit /b
)
 
if "%SENSORES%"=="" set SENSORES=2
if "%ATUADORES%"=="" set ATUADORES=2
if "%CLIENTES%"=="" set CLIENTES=1
 
echo Subindo componentes para o integrador em %IP%...
echo.
 
:: Sensores de temperatura
for /L %%i in (1,1,%SENSORES%) do (
    start "Sensor Temperatura %%i" cmd /k "cd /d %~dp0sensor_temperatura && go run . %IP%"
    echo Sensor temperatura %%i iniciado
)
 
:: Sensores de umidade
for /L %%i in (1,1,%SENSORES%) do (
    start "Sensor Umidade %%i" cmd /k "cd /d %~dp0sensor_umidade && go run . %IP%"
    echo Sensor umidade %%i iniciado
)
 
:: Atuadores
for /L %%i in (1,1,%ATUADORES%) do (
    start "Atuador %%i" cmd /k "cd /d %~dp0atuador && go run . %IP%"
    echo Atuador %%i iniciado
)
 
:: Clientes
for /L %%i in (1,1,%CLIENTES%) do (
    start "Cliente %%i" cmd /k "cd /d %~dp0cliente && go run . %IP%"
    echo Cliente %%i iniciado
)
 
echo.
echo Todos os componentes iniciados.
echo Para encerrar: feche as janelas ou rode derrubar.bat
 

