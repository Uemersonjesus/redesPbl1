
@echo off
set IP=%1
set SENSORES=%2
set ATUADORES=%3
set CLIENTES=%4

if "%IP%"=="" (
    echo Uso: subir.bat ^<IP_DO_INTEGRADOR^> ^<Qtd_Sensores^> ^<Qtd_Atuadores^> ^<Qtd_Clientes^>
    pause
    exit /b
)

:: Define padrões se não forem passados
if "%SENSORES%"=="" set SENSORES=2
if "%ATUADORES%"=="" set ATUADORES=2
if "%CLIENTES%"=="" set CLIENTES=1

echo Iniciando componentes para o integrador em %IP%...

:: Sensores de Temperatura
for /L %%i in (1,1,%SENSORES%) do (
    start "Sensor Temp %%i" cmd /k "sensor_temperatura\sensor.exe %IP%"
)

:: Sensores de Umidade
for /L %%i in (1,1,%SENSORES%) do (
    start "Sensor Umid %%i" cmd /k "sensor_umidade\sensor.exe %IP%"
)

:: Atuadores
for /L %%i in (1,1,%ATUADORES%) do (
    start "Atuador %%i" cmd /k "atuador\atuador.exe %IP%"
)

:: Clientes
for /L %%i in (1,1,%CLIENTES%) do (
    start "Cliente %%i" cmd /k "cliente\client.exe %IP%"
)

echo Concluido. Verifique as janelas abertas.
