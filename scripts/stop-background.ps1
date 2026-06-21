[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$pidFile = Join-Path $projectRoot ".run\opencode.pid"

if (-not (Test-Path $pidFile)) {
    Write-Host "OpenCode is not managed by this project."
    exit 0
}

$processId = [int](Get-Content $pidFile -Raw)
$process = Get-Process -Id $processId -ErrorAction SilentlyContinue
if ($null -ne $process) {
    Stop-Process -Id $processId -Force
    try { Wait-Process -Id $processId -Timeout 10 -ErrorAction SilentlyContinue } catch {}
    Write-Host "Stopped OpenCode (PID $processId)."
} else {
    Write-Host "Removed stale OpenCode PID file."
}
Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
