[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$Port = 4096,

    [string]$Hostname = "127.0.0.1",

    [switch]$NoPure,

    [ValidateRange(1, 120)]
    [int]$WaitSeconds = 30
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$runDirectory = Join-Path $projectRoot ".run"
$pidFile = Join-Path $runDirectory "opencode.pid"
$stdoutFile = Join-Path $runDirectory "opencode.stdout.log"
$stderrFile = Join-Path $runDirectory "opencode.stderr.log"
$healthUrl = "http://${Hostname}:${Port}/global/health"

function Get-ListeningProcessId([int]$ListenPort) {
    $listener = Get-NetTCPConnection -LocalPort $ListenPort -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -ne $listener) {
        return $listener.OwningProcess
    }

    $pattern = "^\s*TCP\s+\S+:${ListenPort}\s+\S+\s+LISTENING\s+(\d+)\s*$"
    foreach ($line in (& netstat.exe -ano -p TCP)) {
        if ($line -match $pattern) {
            return [int]$Matches[1]
        }
    }
    return $null
}

New-Item -ItemType Directory -Path $runDirectory -Force | Out-Null

if (Test-Path $pidFile) {
    $existingPid = [int](Get-Content $pidFile -Raw)
    $existingProcess = Get-Process -Id $existingPid -ErrorAction SilentlyContinue
    if ($null -ne $existingProcess) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $healthUrl -TimeoutSec 2
            if ($response.StatusCode -eq 200) {
                Write-Host "OpenCode is already running (PID $existingPid) at $healthUrl"
                exit 0
            }
        } catch {
            throw "PID $existingPid is running but OpenCode is not healthy at $healthUrl. Stop it before retrying."
        }
    }
    Remove-Item -LiteralPath $pidFile -Force
}

try {
    $existingResponse = Invoke-WebRequest -UseBasicParsing -Uri $healthUrl -TimeoutSec 2
    if ($existingResponse.StatusCode -eq 200) {
        Write-Host "OpenCode is already running at $healthUrl but is not managed by this project."
        exit 0
    }
} catch {
    # No healthy unmanaged server is using the configured endpoint; start one.
}

$command = Get-Command "opencode.cmd" -ErrorAction SilentlyContinue
if ($null -eq $command) {
    $command = Get-Command "opencode" -ErrorAction SilentlyContinue
}
if ($null -eq $command) {
    throw "Cannot find opencode. Install it and ensure it is available on PATH."
}

$arguments = @(
    "serve",
    "--hostname", $Hostname,
    "--port", $Port,
    "--print-logs"
)
if (-not $NoPure) {
    $arguments += "--pure"
}

$process = Start-Process `
    -FilePath $command.Source `
    -ArgumentList $arguments `
    -WorkingDirectory $projectRoot `
    -WindowStyle Hidden `
    -RedirectStandardOutput $stdoutFile `
    -RedirectStandardError $stderrFile `
    -PassThru

Set-Content -LiteralPath $pidFile -Value $process.Id -NoNewline

$deadline = (Get-Date).AddSeconds($WaitSeconds)
while ((Get-Date) -lt $deadline) {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $healthUrl -TimeoutSec 2
        if ($response.StatusCode -eq 200) {
            $listenerProcessId = Get-ListeningProcessId $Port
            $managedProcessId = if ($null -ne $listenerProcessId) { $listenerProcessId } else { $process.Id }
            Set-Content -LiteralPath $pidFile -Value $managedProcessId -NoNewline
            Write-Host "OpenCode started (PID $managedProcessId) at http://${Hostname}:${Port}"
            Write-Host "Logs: $stdoutFile and $stderrFile"
            exit 0
        }
    } catch {
        Start-Sleep -Milliseconds 500
    }

    if ($process.HasExited) {
        Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
        $details = if (Test-Path $stderrFile) { Get-Content $stderrFile -Raw } else { "No error log was produced." }
        throw "OpenCode exited with code $($process.ExitCode).`n$details"
    }
}

try { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue } finally {
    Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
}
throw "OpenCode did not become healthy within $WaitSeconds seconds. See $stderrFile"
