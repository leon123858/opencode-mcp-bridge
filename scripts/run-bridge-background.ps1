[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$Port = 8080,

    [string]$OpenCodeBaseUrl = "http://127.0.0.1:4096",

    [ValidateRange(1, 120)]
    [int]$WaitSeconds = 30
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$runDirectory = Join-Path $projectRoot ".run"
$pidFile = Join-Path $runDirectory "bridge.pid"
$stdoutFile = Join-Path $runDirectory "bridge.stdout.log"
$stderrFile = Join-Path $runDirectory "bridge.stderr.log"
$binaryDirectory = Join-Path $projectRoot "bin"
$binaryPath = Join-Path $binaryDirectory "opencode-mcp-bridge.exe"
$healthUrl = "http://127.0.0.1:${Port}/healthz"

New-Item -ItemType Directory -Path $runDirectory -Force | Out-Null
if (Test-Path $pidFile) {
    $existingPid = [int](Get-Content $pidFile -Raw)
    if ($null -ne (Get-Process -Id $existingPid -ErrorAction SilentlyContinue)) {
        Write-Host "Bridge is already running (PID $existingPid)."
        exit 0
    }
    Remove-Item -LiteralPath $pidFile -Force
}

New-Item -ItemType Directory -Path $binaryDirectory -Force | Out-Null
& go build -o $binaryPath ./cmd
if ($LASTEXITCODE -ne 0) {
    throw "Failed to build the bridge executable."
}

$env:BRIDGE_LISTEN_ADDRESS = "127.0.0.1:${Port}"
$env:OPENCODE_BASE_URL = $OpenCodeBaseUrl
$process = Start-Process `
    -FilePath $binaryPath `
    -WorkingDirectory $projectRoot `
    -WindowStyle Hidden `
    -RedirectStandardOutput $stdoutFile `
    -RedirectStandardError $stderrFile `
    -PassThru
Set-Content -LiteralPath $pidFile -Value $process.Id -NoNewline

$deadline = (Get-Date).AddSeconds($WaitSeconds)
while ((Get-Date) -lt $deadline) {
    if ($process.HasExited) {
        Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
        $details = if (Test-Path $stderrFile) { Get-Content $stderrFile -Raw } else { "No error log was produced." }
        throw "Bridge exited with code $($process.ExitCode).`n$details"
    }
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $healthUrl -TimeoutSec 2
        if ($response.StatusCode -eq 200) {
            Write-Host "Bridge started (PID $($process.Id)) at http://127.0.0.1:${Port}"
            exit 0
        }
    } catch {
        Start-Sleep -Milliseconds 500
    }
}

try {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    Wait-Process -Id $process.Id -Timeout 10 -ErrorAction SilentlyContinue
} finally {
    Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
}
throw "Bridge did not become healthy within $WaitSeconds seconds. See $stderrFile"
