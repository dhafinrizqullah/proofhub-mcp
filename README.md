# proofhub-mcp

ProofHub API v3 MCP server (stdio) — 22 tools inside todolist + label/timesheet/todolist.

> Source-of-truth: `ph-task-manager` (`internal/proofhub/client.go` 22 ops, `main.go` validation). This MCP is MCP-only (no CLI).

## Tools (22)

**Tasks (7):** `task_list`, `task_get`, `task_create`, `task_update`, `task_delete`, `task_copy`, `task_move`
**Subtasks (5):** `subtask_list`, `subtask_get`, `subtask_create`, `subtask_update`, `subtask_delete`
**Comments (5):** `comment_list`, `comment_get`, `comment_create`, `comment_update`, `comment_delete`
**History (2):** `history_list`, `history_get`
**Extras (3):** `todolist_get`, `label_get`, `timesheet_get`

> Note: Spec lists `label_list`/`timesheet_list` alongside the 22-count math (19+3). This build exposes the 22 per math (only `_get` for label/timesheet, plus `todolist_get`). To expose lists, uncomment `label_list`/`timesheet_list` in `main.go:registerTools` (makes 24). `todolist` management (create/update/delete/list) is intentionally not exposed.

All tools validate `isDigits` (numeric IDs) and `YYYY-MM-DD` dates via JSON schema (`pattern`) and runtime checks. Client validates `User-Agent` as `AppName (email)`, retries 429/5xx with exponential backoff, uses `context.WithTimeout` (30s) and `%w` wrapping.

## Env

```
PROOFHUB_BASE_URL=https://yourcompany.proofhub.com
PROOFHUB_API_KEY=YOUR_API_KEY
PROOFHUB_USER_AGENT=proofhub-mcp (you@example.com)  # format: AppName (email), required by ProofHub (400 if missing)
```

## Binary

```bash
go build -o proofhub-mcp . && ./proofhub-mcp
```

Requires env: `PROOFHUB_BASE_URL`, `PROOFHUB_API_KEY`, `PROOFHUB_USER_AGENT`.

Test stdio:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./proofhub-mcp
```

## Docker (clone → build local → run)

```bash
git clone https://github.com/dhafinrizqullah/proofhub-mcp && cd proofhub-mcp
docker build -t proofhub-mcp:local .
# MCP config (stdio via docker):
```

```json
{
  "mcpServers": {
    "proofhub": {
      "command": "docker",
      "args": ["run","-i","--rm","-e","PROOFHUB_BASE_URL","-e","PROOFHUB_API_KEY","-e","PROOFHUB_USER_AGENT","proofhub-mcp:local"]
    }
  }
}
```

Run manually:

```bash
docker run -i --rm -e PROOFHUB_BASE_URL -e PROOFHUB_API_KEY -e PROOFHUB_USER_AGENT proofhub-mcp:local
```

## Verify

```bash
go vet ./... && go test ./...
docker build -t proofhub-mcp:local .
# list_tools (22) includes task_list, label_get, todolist_get
```

## ProofHub Docs

- https://github.com/ProofHub/api_v3 `sections/tasks.md` (tasks, subtasks, comments, history, copy, move)
- `sections/labels.md`, `sections/timesheets.md`, `sections/time.md`

## License

MIT
