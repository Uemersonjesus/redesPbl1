# subir.ps1 — sobe N instâncias de cada componente
# Uso: .\subir.ps1 -IP 192.168.1.10 -Sensores 3 -Atuadores 3 -Clientes 2

param(
    [Parameter(Mandatory=$true)][string]$IP,
    [int]$Sensores = 2,
    [int]$Atuadores = 2,
    [int]$Clientes = 1
)

$pids = @()

for ($i = 1; $i -le $Sensores; $i++) {
    $p = Start-Process powershell -PassThru -ArgumentList "-NoExit", "-Command", `
        "cd '$PSScriptRoot\sensor_temperatura'; go run . $IP"
    $pids += $p.Id
    Write-Host "Sensor temperatura $i iniciado (PID $($p.Id))"

    $p = Start-Process powershell -PassThru -ArgumentList "-NoExit", "-Command", `
        "cd '$PSScriptRoot\sensor_umidade'; go run . $IP"
    $pids += $p.Id
    Write-Host "Sensor umidade $i iniciado (PID $($p.Id))"
}

for ($i = 1; $i -le $Atuadores; $i++) {
    $p = Start-Process powershell -PassThru -ArgumentList "-NoExit", "-Command", `
        "cd '$PSScriptRoot\atuador'; go run . $IP"
    $pids += $p.Id
    Write-Host "Atuador $i iniciado (PID $($p.Id))"
}

for ($i = 1; $i -le $Clientes; $i++) {
    $p = Start-Process powershell -PassThru -ArgumentList "-NoExit", "-Command", `
        "cd '$PSScriptRoot\cliente'; go run . $IP"
    $pids += $p.Id
    Write-Host "Cliente $i iniciado (PID $($p.Id))"
}

# Salva os PIDs para o script de derrubada
$pids | Out-File "$PSScriptRoot\pids.txt"
Write-Host ""
Write-Host "Total: $($pids.Count) processos iniciados." -ForegroundColor Green
Write-Host "Para derrubar tudo: .\derrubar.ps1" -ForegroundColor Yellow