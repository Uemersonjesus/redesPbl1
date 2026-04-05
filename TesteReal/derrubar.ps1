# derrubar.ps1 — encerra todos os processos iniciados pelo subir.ps1

$arquivo = "$PSScriptRoot\pids.txt"

if (-Not (Test-Path $arquivo)) {
    Write-Host "pids.txt não encontrado. Nada para encerrar." -ForegroundColor Red
    exit
}

$pids = Get-Content $arquivo

foreach ($pid in $pids) {
    try {
        Stop-Process -Id $pid -Force -ErrorAction Stop
        Write-Host "Processo $pid encerrado." -ForegroundColor Green
    } catch {
        Write-Host "Processo $pid já não existe." -ForegroundColor Gray
    }
}

Remove-Item $arquivo
Write-Host ""
Write-Host "Todos os processos encerrados." -ForegroundColor Cyan