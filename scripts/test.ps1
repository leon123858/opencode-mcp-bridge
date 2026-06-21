[CmdletBinding()]
param(
    [ValidateRange(0, 100)]
    [double]$MinimumCoverage = 80
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$runDirectory = Join-Path $projectRoot ".run"
$coverageProfile = Join-Path $runDirectory "coverage.out"
$env:GOCACHE = Join-Path $projectRoot ".cache\go-build"
$packages = @(
    "./client",
    "./config",
    "./handlers",
    "./mcpbridge"
)

New-Item -ItemType Directory -Path $runDirectory -Force | Out-Null
Push-Location $projectRoot
try {
    & go test -coverprofile $coverageProfile @packages
    if ($LASTEXITCODE -ne 0) {
        throw "Go tests failed."
    }

    $coverageReport = & go tool cover -func $coverageProfile
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to calculate test coverage."
    }
    $totalLine = $coverageReport | Where-Object { $_ -match '^total:' } | Select-Object -Last 1
    if ($null -eq $totalLine -or $totalLine -notmatch '([0-9]+(?:\.[0-9]+)?)%\s*$') {
        throw "Could not parse total test coverage."
    }

    $totalCoverage = [double]::Parse($Matches[1], [Globalization.CultureInfo]::InvariantCulture)
    Write-Host ("Total coverage: {0:N1}% (minimum: {1:N1}%)" -f $totalCoverage, $MinimumCoverage)
    if ($totalCoverage -lt $MinimumCoverage) {
        throw ("Test coverage {0:N1}% is below the required {1:N1}%." -f $totalCoverage, $MinimumCoverage)
    }
} finally {
    Pop-Location
}
