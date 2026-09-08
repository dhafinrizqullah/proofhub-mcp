# AGENTS.md — proofhub-mcp

Instructions for AI agents working in this repository. Follow this to avoid breaking existing patterns.

## Secret hygiene (MANDATORY, no exceptions)

- NEVER read, open, `cat`, or edit `.env`.
- NEVER print secrets (`PROOFHUB_API_KEY`, tokens) to output, logs, diffs, or example commands.
- NEVER `echo $PROOFHUB_API_KEY` or similar.
- Only read `.env.example` as reference for variable names.
- For verification without credentials, use dummy values: `PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy`

## Go stack (MANDATORY)

- **Go 1.26** (`go.mod:1` `go 1.26`). Module `github.com/dhafinrizqullah/proofhub-mcp`.
- **MCP SDK:** `github.com/mark3labs/mcp-go v1.0.0` (native MCP, not a CLI wrapper).
- Before any Go change, load skills from `https://github.com/samber/cc-skills-golang`:
  - ALWAYS `golang-how-to` first (orchestrator).
  - Then `golang-testing` + `golang-stretchr-testify` for tests, `golang-context` for timeouts, `golang-error-handling` for `%w`, `golang-code-style`/`golang-naming` for style, `golang-security` for secrets/input.
- After change: `go vet ./... && go test ./... -race && go build -o /tmp/proofhub-mcp .`

## Single-todolist scope (CRITICAL — do not break)

- This MCP is **scoped to one todolist only** via ENV:
  ```bash
  PROOFHUB_PROJECT_ID=8213786200   # digits only
  PROOFHUB_TODOLIST_ID=271478716253 # digits only
  ```
- `main.go:loadConfig` + `validateConfig` fail-fast at startup if missing/invalid (lowercase error, no secret leak: `proofhub_project_id is required and must be digits, got ""`).
- **All 24 tools have NO `project_id`/`todolist_id` in input schema** — they are injected from ENV in handlers (`registerTools(s, client, projectID, todolistID)` → `handleTaskList(client, projectID, todolistID)`). Do NOT re-add those params.
- `task_copy`/`task_move` intentionally do NOT allow `new_project_id`/`new_todolist_id` — they stay inside the same todolist (single-scope). Do not expose destination project/list.
- `todolist_get` has **no input** (read-only ENV todolist). `timesheet_list`/`timesheet_get` inject `projectID` from ENV; `label_list`/`label_get` are global (no project).

## ProofHub API v3 handling (must preserve)

Ref: `pkg/proofhub/*.go` — Docs `https://github.com/ProofHub/api_v3`.

| Requirement | Implementation | Where |
|---|---|---|
| **JSON** — `Content-Type: application/json` on POST/PUT, 415 on invalid | `json.Marshal` + `Content-Type` header only when body != nil, `Accept: application/json` always | `client.go:182,211` |
| **User-Agent** — `AppName (email)`, 400 if missing | `userAgentPattern` `^.+\s\(.+@.+\)$`, `Validate()` fail-fast, default `proofhub-mcp (dev@example.com)`, set on every request | `client.go:84,87,210` `main.go:130` |
| **Rate limit** — 429, 25 req/10s, `Retry-After` (seconds or HTTP-date) | `retryableStatus` includes 429, `parseRetryAfter` handles seconds + HTTP-date, capped at 60s, `sleepCtx` respects `ctx` | `client.go:126,148,165,239` |
| **Retry 5xx** — 500/502/503/504 retry after backoff | `retryableStatus` includes 500/502/503/504, `backoff` 1s→2s→4s capped 15s, `MaxRetries=3`, `WithMaxRetries(0)` to opt-out (POST duplicate risk noted) | `client.go:126,138,252` |

- All API calls use `context.WithTimeout(30s)` (`main.go:withTimeout`, `client.go:do` uses `http.NewRequestWithContext`).
- Errors wrapped with `%w` and inspected via `errors.As` (`APIError` at `client.go:101`).

## Project structure

```
proofhub-mcp/
├── main.go              # MCP server, ENV/config, 24 tools, stdio + Streamable HTTP
├── main_test.go         # Unit tests for helpers, config, tool registration (table-driven, testify)
├── pkg/proofhub/
│   ├── client.go        # Core Client, Validate, do, retry, backoff, truncate (268 lines)
│   ├── tasks.go         # Task, Create/Update/Copy/Move types & methods (283 lines)
│   ├── subtasks.go      # Subtask types & methods
│   ├── comments.go      # Comment types & methods
│   ├── history.go       # TaskHistoryEntry/Detail & methods
│   ├── todolist.go      # Todolist + Person & methods
│   ├── labels.go        # Label & methods
│   ├── timesheets.go    # Timesheet/TimeEntry & methods
│   ├── helpers.go       # ParseTarget, digitsOnly, stripPrefixDigits
│   ├── client_test.go   # Client unit tests (httptest, retry, validation)
│   └── helpers_test.go  # ParseTarget helpers tests
├── go.mod (go 1.26)
├── Dockerfile (golang:1.26-alpine → distroless/static:nonroot, EXPOSE 8080)
├── docker-compose.yml (http mode)
├── .env.example (English comments)
└── README.md (English, native MCP, not CLI wrapper)
```

- Do NOT merge these files back into one large file — the split is intentional for readability.
- Keep imports minimal per file (no unused imports).
- Keep all code/comments/README in **English**.

## Tools (24) — descriptions must match README

All `mcp.NewTool(WithDescription("..."))` in `main.go:383` must exactly match `README.md` table `| Tool | Description | ... |`.

| Category | Tools |
|---|---|
| Tasks (7) | `task_list` (no input), `task_get` (`task_id`), `task_create` (`title`+...), `task_update` (`task_id`+...), `task_delete`/`task_copy`/`task_move` (`task_id`) |
| Subtasks (5) | `subtask_list` (`task_id`), `subtask_get`/`subtask_update`/`subtask_delete` (`task_id`+`subtask_id`), `subtask_create` (`task_id`+`title`) |
| Comments (5) | `comment_list` (`task_id`), `comment_get`/`comment_update`/`comment_delete` (`task_id`+`comment_id`), `comment_create` (`task_id`+`description`) |
| History (2) | `history_list` (`task_id`), `history_get` (`task_id`+`history_id`) |
| Meta (5) | `todolist_get` (-), `label_list` (-), `label_get` (`label_id`), `timesheet_list` (-), `timesheet_get` (`timesheet_id`) |

- Input validation: `isDigits` (`^[0-9]+$`), `YYYY-MM-DD` (`time.Parse`), `required` per schema.
- Do NOT add `project_id`/`todolist_id` to any tool — scoped via ENV.

## Transports

- `main.go:loadConfig` supports `MCP_TRANSPORT=stdio|http` (default `stdio`) and `MCP_HTTP_PORT=8080` via ENV or flags `--transport`/`--http-port`.
- `main.go:140` `serveHTTP` uses `server.NewStreamableHTTPServer` on `/:8080/mcp` + `/health` (200 ok). `Dockerfile:9` `EXPOSE 8080`.
- Test both: `go test` + manual `curl http://localhost:8080/health` and `POST /mcp` with `initialize`.

## Testing

- Framework: `github.com/stretchr/testify` (`assert.New(t)` per subtest, `require` for preconditions, `t.Parallel` where safe — never with `t.Setenv`).
- Table-driven with `name` field, `t.Run`.
- `main_test.go` mirrors `main.go` order; `pkg/proofhub/*_test.go` mirrors source file.
- Use `httptest.NewServer` for client tests; verify headers (`X-API-KEY`, `User-Agent`), retry behavior, context cancel.
- Run: `go test ./... -race -cover`.

## Docker (local build only, do NOT push to registry)

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/proofhub-mcp .
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /bin/proofhub-mcp /bin/proofhub-mcp
EXPOSE 8080
ENTRYPOINT ["/bin/proofhub-mcp"]
```

- `.dockerignore` must exclude `.env`.
- `README` must show both: `go build -o proofhub-mcp . && ./proofhub-mcp` and `docker build -t proofhub-mcp:local .` + harness JSON for stdio and http.

## Git

- Branch `main`, remote `https://github.com/dhafinrizqullah/proofhub-mcp.git`.
- Commit style: `feat: ...`, `fix: ...`, `test: ...`, `docs: ...`.
- Push: `git add . && git commit -m "..." && git push -u origin main`.

## Common pitfalls (do NOT do)

- ❌ Reading `.env` or echoing `PROOFHUB_API_KEY`.
- ❌ Adding `project_id`/`todolist_id` to tool inputs (breaks scope).
- ❌ Exposing todolist management (create/list/delete todolists) — only `todolist_get` is allowed.
- ❌ Using Indonesian in README/code/comments (must be English).
- ❌ Merging `pkg/proofhub/*.go` back into one file.
- ❌ Forgetting `context.WithTimeout` or `%w` wrapping.
- ❌ Missing `pattern ^[0-9]+$` or `YYYY-MM-DD` in tool schema.
- ❌ Changing `go.mod` to <1.26.
- ❌ Pushing Docker image to registry (local build only).

## Verify before push

```bash
go vet ./... && go test ./... -race
go build -o /tmp/proofhub-mcp . && docker build -t proofhub-mcp:local .
# stdio
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy PROOFHUB_USER_AGENT="proofhub-mcp (test@example.com)" PROOFHUB_PROJECT_ID=1 PROOFHUB_TODOLIST_ID=1 ./proofhub-mcp
# http
MCP_TRANSPORT=http MCP_HTTP_PORT=8080 PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy PROOFHUB_USER_AGENT="proofhub-mcp (test@example.com)" PROOFHUB_PROJECT_ID=1 PROOFHUB_TODOLIST_ID=1 ./proofhub-mcp & curl http://localhost:8080/health
```
