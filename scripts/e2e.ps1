[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$OpenCodePort = 4096,

    [ValidateRange(1, 65535)]
    [int]$BridgePort = 8080,

    [ValidateRange(1, 120)]
    [int]$WaitSeconds = 30,

    [switch]$KeepRunning
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$runDirectory = Join-Path $projectRoot ".run"
$openCodePidFile = Join-Path $runDirectory "opencode.pid"
$bridgePidFile = Join-Path $runDirectory "bridge.pid"
$openCodeUrl = "http://127.0.0.1:${OpenCodePort}"
$bridgeUrl = "http://127.0.0.1:${BridgePort}"
$mcpProtocolVersion = "2025-11-25"

function Get-ManagedProcessId([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        return $null
    }

    $value = 0
    if (-not [int]::TryParse((Get-Content -LiteralPath $Path -Raw).Trim(), [ref]$value)) {
        return $null
    }
    if ($null -eq (Get-Process -Id $value -ErrorAction SilentlyContinue)) {
        return $null
    }
    return $value
}

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) {
        throw "E2E assertion failed: $Message"
    }
}

function Invoke-McpRequest(
    [int]$Id,
    [string]$Method,
    [hashtable]$Params,
    [int]$TimeoutSeconds = 10
) {
    $body = @{
        jsonrpc = "2.0"
        id = $Id
        method = $Method
        params = $Params
    } | ConvertTo-Json -Depth 10 -Compress

    $response = Invoke-WebRequest `
        -UseBasicParsing `
        -Uri "${bridgeUrl}/mcp" `
        -Method Post `
        -ContentType "application/json" `
        -Headers @{
            Accept = "application/json, text/event-stream"
            "MCP-Protocol-Version" = $mcpProtocolVersion
        } `
        -Body $body `
        -TimeoutSec $TimeoutSeconds

    Assert-True ($response.StatusCode -eq 200) "MCP $Method returned HTTP $($response.StatusCode)"
    return $response.Content | ConvertFrom-Json
}

$initialOpenCodePid = Get-ManagedProcessId $openCodePidFile
$initialBridgePid = Get-ManagedProcessId $bridgePidFile
$startedOpenCode = $false
$startedBridge = $false
$createdSessionIds = @()
$failure = $null

try {
    Write-Host "[e2e] Starting OpenCode..."
    & (Join-Path $PSScriptRoot "run-background.ps1") `
        -Port $OpenCodePort `
        -Hostname "127.0.0.1" `
        -WaitSeconds $WaitSeconds
    $currentOpenCodePid = Get-ManagedProcessId $openCodePidFile
    $startedOpenCode = ($null -ne $currentOpenCodePid -and $currentOpenCodePid -ne $initialOpenCodePid)

    Write-Host "[e2e] Starting bridge..."
    & (Join-Path $PSScriptRoot "run-bridge-background.ps1") `
        -Port $BridgePort `
        -OpenCodeBaseUrl $openCodeUrl `
        -WaitSeconds $WaitSeconds
    $currentBridgePid = Get-ManagedProcessId $bridgePidFile
    $startedBridge = ($null -ne $currentBridgePid -and $currentBridgePid -ne $initialBridgePid)

    Write-Host "[e2e] Checking bridge health..."
    $health = Invoke-RestMethod -Uri "${bridgeUrl}/healthz" -TimeoutSec 10
    Assert-True ($health.status -eq "ok") "GET /healthz did not return status=ok"

    Write-Host "[e2e] Checking OpenCode proxy..."
    $setup = Invoke-RestMethod -Uri "${bridgeUrl}/opencode/setup" -TimeoutSec 10
    Assert-True ($setup.healthy -eq $true) "GET /opencode/setup did not report a healthy OpenCode server"

    Write-Host "[e2e] Initializing MCP transport..."
    $initialize = Invoke-McpRequest 1 "initialize" @{
        protocolVersion = $mcpProtocolVersion
        capabilities = @{}
        clientInfo = @{ name = "make-e2e"; version = "1.0.0" }
    }
    Assert-True ($null -eq $initialize.error) "MCP initialize returned an error"
    Assert-True ($initialize.result.serverInfo.name -eq "opencode-mcp-bridge") "MCP initialize returned an unexpected server name"

    Write-Host "[e2e] Checking MCP tool discovery..."
    $toolsResponse = Invoke-McpRequest 2 "tools/list" @{}
    Assert-True ($null -eq $toolsResponse.error) "MCP tools/list returned an error"
    $toolNames = @($toolsResponse.result.tools | ForEach-Object { $_.name })
    $expectedTools = @(
        "opencode_setup",
        "opencode_ask",
        "opencode_reply",
        "opencode_run",
        "opencode_check",
        "opencode_conversation",
        "opencode_sessions_overview",
        "opencode_mcp_servers",
        "opencode_provider_test"
    )
    foreach ($toolName in $expectedTools) {
        Assert-True ($toolNames -contains $toolName) "MCP tools/list is missing $toolName"
    }

    Write-Host "[e2e] Calling MCP setup tool..."
    $setupTool = Invoke-McpRequest 3 "tools/call" @{
        name = "opencode_setup"
        arguments = @{}
    }
    Assert-True ($null -eq $setupTool.error) "MCP opencode_setup returned a protocol error"
    Assert-True ($setupTool.result.isError -ne $true) "MCP opencode_setup returned a tool error"
    $setupToolPayload = $setupTool.result.content[0].text | ConvertFrom-Json
    Assert-True ($setupToolPayload.healthy -eq $true) "MCP opencode_setup did not report a healthy OpenCode server"

    Write-Host "[e2e] Selecting a connected provider and model..."
    $providerCatalog = Invoke-RestMethod -Uri "${openCodeUrl}/provider" -TimeoutSec 10
    $connectedProviderIds = @($providerCatalog.connected)
    Assert-True ($connectedProviderIds.Count -gt 0) "OpenCode has no connected provider for conversational E2E tests"
    if ($connectedProviderIds -contains "opencode") {
        $connectedProviderIds = @("opencode") + @($connectedProviderIds | Where-Object { $_ -ne "opencode" })
    }
    $selectedProvider = $null
    $selectedModelId = $null
    foreach ($providerId in $connectedProviderIds) {
        $candidate = @($providerCatalog.all | Where-Object { $_.id -eq $providerId } | Select-Object -First 1)
        if ($candidate.Count -eq 0) {
            continue
        }
        $defaultModelProperty = $providerCatalog.default.PSObject.Properties[$providerId]
        if ($null -ne $defaultModelProperty -and $candidate[0].models.PSObject.Properties.Name -contains $defaultModelProperty.Value) {
            $selectedProvider = $candidate[0]
            $selectedModelId = $defaultModelProperty.Value
            break
        }
    }
    Assert-True ($null -ne $selectedProvider -and -not [string]::IsNullOrWhiteSpace($selectedModelId)) "No connected provider exposes an available model"
    Write-Host "[e2e] Using provider $($selectedProvider.id), model $selectedModelId."

    Write-Host "[e2e] Testing opencode_provider_test..."
    $providerTool = Invoke-McpRequest 4 "tools/call" @{
        name = "opencode_provider_test"
        arguments = @{
            providerId = $selectedProvider.id
            modelID = $selectedModelId
        }
    } 120
    Assert-True ($null -eq $providerTool.error) "MCP opencode_provider_test returned a protocol error"
    Assert-True ($providerTool.result.isError -ne $true) "MCP opencode_provider_test returned a tool error"
    $providerPayload = $providerTool.result.content[0].text | ConvertFrom-Json
    Assert-True (-not [string]::IsNullOrWhiteSpace($providerPayload.sessionId)) "MCP opencode_provider_test did not return a session ID"
    $createdSessionIds += $providerPayload.sessionId
    Assert-True ($providerPayload.available -eq $true) "MCP opencode_provider_test did not report available=true"

    Write-Host "[e2e] Testing opencode_ask..."
    $askMarker = "OPENCODE_BRIDGE_ASK_OK"
    $askTool = Invoke-McpRequest 5 "tools/call" @{
        name = "opencode_ask"
        arguments = @{
            prompt = "Reply with the exact marker ${askMarker}."
            providerID = $selectedProvider.id
            modelID = $selectedModelId
        }
    } 120
    Assert-True ($null -eq $askTool.error) "MCP opencode_ask returned a protocol error"
    Assert-True ($askTool.result.isError -ne $true) "MCP opencode_ask returned a tool error"
    $askPayload = $askTool.result.content[0].text | ConvertFrom-Json
    Assert-True (-not [string]::IsNullOrWhiteSpace($askPayload.sessionId)) "MCP opencode_ask did not return a session ID"
    $conversationSessionId = $askPayload.sessionId
    $createdSessionIds += $conversationSessionId
    Assert-True (-not [string]::IsNullOrWhiteSpace($askPayload.message)) "MCP opencode_ask returned an empty assistant response"

    Write-Host "[e2e] Testing opencode_reply..."
    $replyMarker = "OPENCODE_BRIDGE_REPLY_OK"
    $replyTool = Invoke-McpRequest 6 "tools/call" @{
        name = "opencode_reply"
        arguments = @{
            sessionId = $conversationSessionId
            prompt = "Reply with the exact marker ${replyMarker}."
            providerID = $selectedProvider.id
            modelID = $selectedModelId
        }
    } 120
    Assert-True ($null -eq $replyTool.error) "MCP opencode_reply returned a protocol error"
    Assert-True ($replyTool.result.isError -ne $true) "MCP opencode_reply returned a tool error"
    $replyPayload = $replyTool.result.content[0].text | ConvertFrom-Json
    Assert-True ($replyPayload.sessionId -eq $conversationSessionId) "MCP opencode_reply returned the wrong session ID"
    Assert-True (-not [string]::IsNullOrWhiteSpace($replyPayload.message)) "MCP opencode_reply returned an empty assistant response"

    Write-Host "[e2e] Testing opencode_check..."
    $checkTool = Invoke-McpRequest 7 "tools/call" @{
        name = "opencode_check"
        arguments = @{
            sessionId = $conversationSessionId
            detailed = $true
        }
    }
    Assert-True ($null -eq $checkTool.error) "MCP opencode_check returned a protocol error"
    Assert-True ($checkTool.result.isError -ne $true) "MCP opencode_check returned a tool error"
    $checkPayload = $checkTool.result.content[0].text | ConvertFrom-Json
    Assert-True ($checkPayload.session.id -eq $conversationSessionId) "MCP opencode_check returned the wrong session"
    Assert-True ($checkPayload.PSObject.Properties.Name -contains "status") "MCP opencode_check did not return status"
    Assert-True ($checkPayload.PSObject.Properties.Name -contains "todos") "MCP opencode_check detailed result did not return todos"
    Assert-True ($checkPayload.PSObject.Properties.Name -contains "diff") "MCP opencode_check detailed result did not return diff"

    Write-Host "[e2e] Testing opencode_conversation..."
    $conversationTool = Invoke-McpRequest 8 "tools/call" @{
        name = "opencode_conversation"
        arguments = @{
            sessionId = $conversationSessionId
            limit = 20
        }
    }
    Assert-True ($null -eq $conversationTool.error) "MCP opencode_conversation returned a protocol error"
    Assert-True ($conversationTool.result.isError -ne $true) "MCP opencode_conversation returned a tool error"
    $conversationPayload = $conversationTool.result.content[0].text | ConvertFrom-Json
    $conversationText = $conversationPayload | ConvertTo-Json -Depth 20 -Compress
    Assert-True ($conversationText.Contains($askMarker)) "MCP opencode_conversation is missing the ask response"
    Assert-True ($conversationText.Contains($replyMarker)) "MCP opencode_conversation is missing the reply response"
    $userMessages = @($conversationPayload | Where-Object { $_.info.role -eq "user" })
    $assistantMessages = @($conversationPayload | Where-Object { $_.info.role -eq "assistant" })
    Assert-True ($userMessages.Count -ge 2) "MCP opencode_conversation did not return both user prompts"
    Assert-True ($assistantMessages.Count -ge 2) "MCP opencode_conversation did not return both assistant responses"

    Write-Host "[e2e] Testing opencode_sessions_overview..."
    $sessionsTool = Invoke-McpRequest 9 "tools/call" @{
        name = "opencode_sessions_overview"
        arguments = @{}
    }
    Assert-True ($null -eq $sessionsTool.error) "MCP opencode_sessions_overview returned a protocol error"
    Assert-True ($sessionsTool.result.isError -ne $true) "MCP opencode_sessions_overview returned a tool error"
    $sessionsPayload = @($sessionsTool.result.content[0].text | ConvertFrom-Json)
    $sessionMatch = @($sessionsPayload | Where-Object { $_.id -eq $conversationSessionId })
    Assert-True ($sessionMatch.Count -eq 1) "MCP opencode_sessions_overview did not include the test session"

    Write-Host "[e2e] Testing opencode_mcp_servers..."
    $mcpServersTool = Invoke-McpRequest 10 "tools/call" @{
        name = "opencode_mcp_servers"
        arguments = @{}
    }
    Assert-True ($null -eq $mcpServersTool.error) "MCP opencode_mcp_servers returned a protocol error"
    Assert-True ($mcpServersTool.result.isError -ne $true) "MCP opencode_mcp_servers returned a tool error"
    $null = $mcpServersTool.result.content[0].text | ConvertFrom-Json

    Write-Host "[e2e] Running a complex OpenCode task through the bridge..."
    $taskMarker = "OPENCODE_BRIDGE_COMPLEX_TASK_OK"
    $taskPrompt = @"
Perform a read-only integration audit of the current repository. You must use repository tools to inspect go.mod, Makefile, and server/server.go. Do not modify any file.

Return a concise report that includes all of these exact strings:
- $taskMarker
- github.com/modelcontextprotocol/go-sdk
- github.com/labstack/echo/v4
- /healthz
- /mcp
- /opencode/ask
- make check
- make e2e

For each item, briefly state what you verified from the files. Do not merely repeat this prompt; base the report on the repository contents.
"@
    $taskTool = Invoke-McpRequest 11 "tools/call" @{
        name = "opencode_run"
        arguments = @{
            prompt = $taskPrompt
            maxDurationSeconds = 300
            providerID = $selectedProvider.id
            modelID = $selectedModelId
        }
    } 330
    Assert-True ($null -eq $taskTool.error) "MCP opencode_run returned a protocol error"
    Assert-True ($taskTool.result.isError -ne $true) "MCP opencode_run returned a tool error"
    $taskPayload = $taskTool.result.content[0].text | ConvertFrom-Json
    Assert-True ($taskPayload.status -eq "completed") "MCP opencode_run did not complete"
    Assert-True (-not [string]::IsNullOrWhiteSpace($taskPayload.sessionId)) "MCP opencode_run did not return a session ID"
    $createdSessionIds += $taskPayload.sessionId
    foreach ($expectedText in @(
        $taskMarker,
        "github.com/modelcontextprotocol/go-sdk",
        "github.com/labstack/echo/v4",
        "/healthz",
        "/mcp",
        "/opencode/ask",
        "make check",
        "make e2e"
    )) {
        Assert-True ($taskPayload.message.Contains($expectedText)) "Complex task response is missing: $expectedText"
    }

    Write-Host "[e2e] PASS - REST proxy, MCP transport, and complex OpenCode task execution are operational."
} catch {
    $failure = $_
} finally {
    foreach ($sessionId in @($createdSessionIds | Select-Object -Unique)) {
        if ([string]::IsNullOrWhiteSpace($sessionId)) {
            continue
        }
        try {
            Invoke-WebRequest `
                -UseBasicParsing `
                -Uri "${openCodeUrl}/session/${sessionId}" `
                -Method Delete `
                -TimeoutSec 10 | Out-Null
            Write-Host "[e2e] Removed test session $sessionId."
        } catch {
            Write-Warning "Failed to remove test session ${sessionId}: $($_.Exception.Message)"
        }
    }

    # Startup helpers can fail after creating a process. Re-check the PID files
    # here so those partial starts are still cleaned up.
    $finalBridgePid = Get-ManagedProcessId $bridgePidFile
    $finalOpenCodePid = Get-ManagedProcessId $openCodePidFile
    $startedBridge = $startedBridge -or ($null -ne $finalBridgePid -and $finalBridgePid -ne $initialBridgePid)
    $startedOpenCode = $startedOpenCode -or ($null -ne $finalOpenCodePid -and $finalOpenCodePid -ne $initialOpenCodePid)

    if (-not $KeepRunning) {
        if ($startedBridge) {
            try {
                & (Join-Path $PSScriptRoot "stop-bridge-background.ps1")
            } catch {
                Write-Warning "Failed to stop bridge: $($_.Exception.Message)"
            }
        }
        if ($startedOpenCode) {
            try {
                & (Join-Path $PSScriptRoot "stop-background.ps1")
            } catch {
                Write-Warning "Failed to stop OpenCode: $($_.Exception.Message)"
            }
        }
    } else {
        Write-Host "[e2e] Services left running because -KeepRunning was specified."
    }
}

if ($null -ne $failure) {
    throw $failure
}
