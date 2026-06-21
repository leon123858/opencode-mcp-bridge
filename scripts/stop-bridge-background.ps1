[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$pidFile = Join-Path $projectRoot ".run\bridge.pid"

if (-not (Test-Path $pidFile)) {
    Write-Host "Bridge is not managed by this project."
    exit 0
}
$processId = [int](Get-Content $pidFile -Raw)
if ($null -ne (Get-Process -Id $processId -ErrorAction SilentlyContinue)) {
    Stop-Process -Id $processId -Force
    try { Wait-Process -Id $processId -Timeout 10 -ErrorAction SilentlyContinue } catch {}
    Write-Host "Stopped bridge (PID $processId)."
}
Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
