# opencode MCP bridge

以 Go 實作的 HTTP bridge，讓 MCP 客戶端透過 Streamable HTTP 或 REST API 使用既有的 OpenCode 伺服器。Bridge **不會**自行啟動 OpenCode。

## 快速開始

需要 Go 1.25+ 與 `opencode` CLI。先啟動 OpenCode，再啟動 bridge：

```powershell
opencode serve --port 4096
$env:OPENCODE_BASE_URL = "http://127.0.0.1:4096"
go run ./cmd
```

Bridge 預設監聽 `:8080`。可用環境變數：

- `BRIDGE_LISTEN_ADDRESS`：監聽位址，預設 `:8080`
- `BRIDGE_REQUEST_TIMEOUT`：上游 HTTP timeout，預設 `30s`
- `OPENCODE_BASE_URL`：OpenCode server URL，預設 `http://127.0.0.1:4096`
- `OPENCODE_SERVER_USERNAME` / `OPENCODE_SERVER_PASSWORD`：HTTP Basic Auth

## MCP 連線

Streamable HTTP endpoint 為 `http://localhost:8080/mcp`（根路徑 `/` 亦可使用）。客戶端設定範例：

```json
{
  "mcpServers": {
    "opencode": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

Bridge 提供九個工具：

- `opencode_setup`
- `opencode_ask`
- `opencode_reply`
- `opencode_run`
- `opencode_check`
- `opencode_conversation`
- `opencode_sessions_overview`
- `opencode_mcp_servers`
- `opencode_provider_test`

## REST API

| Method | Path | 功能 |
|---|---|---|
| GET | `/opencode/setup` | 檢查服務與目前專案 |
| POST | `/opencode/ask` | 建立 session 並提問 |
| POST | `/opencode/reply` | 對既有 session 追問 |
| POST | `/opencode/run` | 執行任務並等待完成 |
| GET | `/opencode/check?sessionId=...` | 查詢 session 狀態 |
| GET | `/opencode/conversation?sessionId=...&limit=...` | 取得對話歷史 |
| GET | `/opencode/sessions-overview` | 列出 sessions |
| GET | `/opencode/mcp-servers` | 查詢 OpenCode MCP servers |
| POST | `/opencode/provider-test` | 測試 provider/model |

REST 呼叫範例：

```powershell
Invoke-RestMethod http://localhost:8080/opencode/ask `
  -Method Post -ContentType application/json `
  -Body '{"prompt":"Explain this project"}'
```

若無法連上 OpenCode，API 會回傳 HTTP 503：

```json
{
  "error": "cannot connect to OpenCode server",
  "hint": "請先執行 `opencode serve` 啟動 OpenCode 伺服器"
}
```

## 驗證

```powershell
make check
make e2e
```

`make check` 會依序執行格式化、vet、單元測試與 build。`make e2e` 需要 OpenCode 已配置可用 provider/model；它會啟動 OpenCode 與 bridge，實際呼叫全部九個 MCP tools，包含 ask/reply 對話鏈及透過 `opencode_run` 執行的跨檔案稽核任務，最後刪除所有測試 sessions 並清理背景程序。

`make test` 會產生 `.run/coverage.out`，並要求核心套件的整體 statement coverage 不低於 80%。
