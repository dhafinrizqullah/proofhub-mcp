# ProofHub MCP

ProofHub MCP — MCP server implementation of ProofHub API v3 (https://github.com/ProofHub/api_v3) for task management. Native MCP (stdio & Streamable HTTP), not a CLI wrapper. Locked to a **single project** with a configurable set of **maintainable todolists** (`PROOFHUB_PROJECT_ID` + `PROOFHUB_TODOLIST_IDS` allowlist) — no tool can access other projects or unlisted lists.

## Environment

**Required:**
```bash
PROOFHUB_BASE_URL=https://yourcompany.proofhub.com
PROOFHUB_API_KEY=YOUR_API_KEY
PROOFHUB_USER_AGENT=proofhub-mcp (you@example.com)  # format AppName (email), ProofHub returns 400 without it
PROOFHUB_PROJECT_ID=8213786200                          # digits only, fail-fast if empty/invalid
PROOFHUB_TODOLIST_IDS=271478716253,271478716254            # comma-separated allowlist of maintainable lists
```

**Optional (transport):**
```bash
MCP_TRANSPORT=stdio  # stdio (default) or http
MCP_HTTP_PORT=8080   # port for http, default 8080
```

ENV validation runs at startup — lowercase errors without leaking secrets (`proofhub_project_id is required and must be digits, got ""`).

> The legacy single `PROOFHUB_TODOLIST_ID` still works as a one-element allowlist for backward compatibility. When only one list is configured, `todolist_id` may be omitted per call; with several lists it is required and must be in the allowlist, otherwise the tool returns which IDs are maintainable.

### How to get your API key

1. Log in to your ProofHub account (`https://yourcompany.proofhub.com`).
2. Click your profile picture/avatar in the bottom-left corner.
3. Open **Profile** → **API Access** menu.
4. Copy the API key (used as `X-API-KEY` / `PROOFHUB_API_KEY`). Keep it secret — never commit it to git.

> Tip: On some ProofHub versions you need to click the profile picture 5 times to reveal the API key field.

## Tools (26) — single project, maintainable todolists

All task/subtask/comment/history tools operate strictly in `PROOFHUB_PROJECT_ID` and require `todolist_id` from the `PROOFHUB_TODOLIST_IDS` allowlist (omit only when one list is configured). Nothing escapes ENV: there is no `project_id` param on any tool. `todolist_list` returns the maintainable lists, `label_*` and `people_list` are global, `timesheet_*` are project-scoped.

| Tool | Description | Input | Output |
|------|-------------|-------|--------|
| `task_list` | List all tasks in a maintainable todolist. Single project (ENV); todolist_id must be one of PROOFHUB_TODOLIST_IDS (omit only when one list is configured). | `todolist_id` | `Task[]` JSON |
| `task_get` | Get a single task in a maintainable todolist. Requires task_id (digits) and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id` (digits), `todolist_id` | `Task` JSON |
| `task_create` | Create a new task in a maintainable todolist. Requires title and todolist_id from PROOFHUB_TODOLIST_IDS. | `title` (req), `todolist_id`, `description`, `start_date`/`due_date` (YYYY-MM-DD), `estimated_hours`/`mins`, `assigned` (int[]), `labels` (int[]) | `Task` JSON |
| `task_update` | Update a task in a maintainable todolist. Requires task_id and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id` (req), `todolist_id`, `title`, `description`, `start_date`, `due_date`, `estimated_hours`/`mins`, `logged_hours`/`mins`, `percent_progress` 0-100, `assigned`, `labels`, `completed` (bool), `stage_id` | `Task` JSON |
| `task_delete` | Delete a task in a maintainable todolist. Requires task_id and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `todolist_id` | `deleted task <id>` text |
| `task_copy` | Copy (duplicate) a task between maintainable todolists (same project). todolist_id and new_todolist_id must be in PROOFHUB_TODOLIST_IDS; destination defaults to source. | `task_id` (req), `todolist_id`, `new_todolist_id`, `title`, `stage_id`, `copy_assignees`/`copy_custom_fields`/`copy_dates`/`copy_comments` (bool) | `Task` JSON |
| `task_move` | Move a task between maintainable todolists (same project). todolist_id and new_todolist_id must be in PROOFHUB_TODOLIST_IDS; destination defaults to source. | `task_id` (req), `todolist_id`, `new_todolist_id`, `title`, `stage_id`, `move_people`, `copy_assignees`/`custom_fields`, `move_dates`, `proof_comment`, `copy_comments`, `completed` | `Task` JSON |
| `subtask_list` | List subtasks under a task in a maintainable todolist. Requires task_id and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `todolist_id` | `Subtask[]` JSON |
| `subtask_get` | Get a single subtask in a maintainable todolist. Requires task_id, subtask_id, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `subtask_id`, `todolist_id` | `Subtask` JSON |
| `subtask_create` | Create a new subtask in a maintainable todolist. Requires task_id, title, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `title` (req), `todolist_id`, `description`, `start_date`/`due_date`, `estimated_hours`/`mins`, `assigned`, `labels` | `Subtask` JSON |
| `subtask_update` | Update a subtask in a maintainable todolist. Requires task_id, subtask_id, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `subtask_id`, `todolist_id`, `title`, `description`, `start_date`/`due_date`, `estimated_hours`/`mins`, `assigned`, `labels`, `completed` | `Subtask` JSON |
| `subtask_delete` | Delete a subtask in a maintainable todolist. Requires task_id, subtask_id, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `subtask_id`, `todolist_id` | text |
| `comment_list` | List comments on a task in a maintainable todolist. Requires task_id and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `todolist_id` | `Comment[]` JSON |
| `comment_get` | Get a single comment in a maintainable todolist. Requires task_id, comment_id, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `comment_id`, `todolist_id` | `Comment` JSON |
| `comment_create` | Create a comment on a task in a maintainable todolist. Requires task_id, description, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `description` (req), `todolist_id` | `Comment` JSON |
| `comment_update` | Update a comment in a maintainable todolist. Requires task_id, comment_id, description, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `comment_id`, `description` (req), `todolist_id` | `Comment` JSON |
| `comment_delete` | Delete a comment in a maintainable todolist. Requires task_id, comment_id, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `comment_id`, `todolist_id` | text |
| `history_list` | List audit history for a task in a maintainable todolist. Requires task_id and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `todolist_id` | `TaskHistoryEntry[]` JSON |
| `history_get` | Get a single history entry in a maintainable todolist. Requires task_id, history_id, and todolist_id from PROOFHUB_TODOLIST_IDS. | `task_id`, `history_id`, `todolist_id` | `TaskHistoryDetail` JSON |
| `todolist_get` | Get maintainable todolist details. todolist_id must be in PROOFHUB_TODOLIST_IDS (omit only when one list is configured). | `todolist_id` | `Todolist` JSON |
| `todolist_list` | List the maintainable todolists configured via PROOFHUB_TODOLIST_IDS (single project). No input required. | - | `Todolist[]` JSON |
| `label_list` | List all labels (global, not scoped to todolist). No input required. Returns id, name, color. | - | `Label[]` JSON |
| `label_get` | Get a single label by ID. Requires label_id (digits). Global lookup, not bound to ENV todolist. | `label_id` (digits) | `Label` JSON |
| `timesheet_list` | List timesheets in the configured project (PROOFHUB_PROJECT_ID). No input required. | - | `Timesheet[]` JSON |
| `timesheet_get` | Get a single timesheet in the configured project. Requires timesheet_id (digits). | `timesheet_id` (digits) | `Timesheet` JSON |
| `people_list` | List all people in ProofHub account (global). Use to lookup assignee IDs for task_create/task_update/subtask. Returns id, first_name, last_name, email. No input required. | - | `Person[]` JSON |
> All `WithDescription` strings in `main.go` exactly match the Description column above. Input validation: `isDigits` for all IDs (`pattern ^[0-9]+$`), `YYYY-MM-DD` for dates, `required` per schema.

## Assignees

Assignee IDs are `people_id` values discovered via `people_list`. Before calling `task_create`, `task_update`, `subtask_create`, or `subtask_update`, call `people_list` to lookup users by `first_name`/`last_name`/`email` and use their `id` in `assigned`.

Example:

```json
{
  "title": "Fix onboarding bug",
  "assigned": [9526247227]
}
```

Lookup flow: `people_list` → find `id` for `first_name`/`email` → `task_create` with `assigned: [9526247227]`. `people_list` is global (not scoped to `PROOFHUB_PROJECT_ID`/`PROOFHUB_TODOLIST_IDS`) — no input required, no filtering by project.

## Binary

```bash
go build -o proofhub-mcp . && ./proofhub-mcp
# or with flags
./proofhub-mcp --transport http --http-port 8080
```

Requires mandatory ENV. Example stdio:

```bash
export PROOFHUB_BASE_URL=https://yourcompany.proofhub.com
export PROOFHUB_API_KEY=xxx
export PROOFHUB_USER_AGENT="proofhub-mcp (you@example.com)"
export PROOFHUB_PROJECT_ID=8213786200
export PROOFHUB_TODOLIST_IDS=271478716253,271478716254
./proofhub-mcp
# transport defaults to stdio
```

## Docker (clone → build local → run)

```bash
git clone https://github.com/dhafinrizqullah/proofhub-mcp && cd proofhub-mcp
docker build -t proofhub-mcp:local .
```

### a) stdio (default)

```bash
docker run -i --rm \
  -e PROOFHUB_BASE_URL \
  -e PROOFHUB_API_KEY \
  -e PROOFHUB_USER_AGENT \
  -e PROOFHUB_PROJECT_ID \
  -e PROOFHUB_TODOLIST_IDS \
  proofhub-mcp:local
```

MCP harness config (stdio via docker):

```json
{
  "mcpServers": {
    "proofhub": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "PROOFHUB_BASE_URL",
        "-e", "PROOFHUB_API_KEY",
        "-e", "PROOFHUB_USER_AGENT",
        "-e", "PROOFHUB_PROJECT_ID",
        "-e", "PROOFHUB_TODOLIST_IDS",
        "proofhub-mcp:local"
      ]
    }
  }
}
```

### b) http (Streamable HTTP)

```bash
docker run -p 8080:8080 \
  -e MCP_TRANSPORT=http \
  -e MCP_HTTP_PORT=8080 \
  -e PROOFHUB_BASE_URL \
  -e PROOFHUB_API_KEY \
  -e PROOFHUB_USER_AGENT \
  -e PROOFHUB_PROJECT_ID \
  -e PROOFHUB_TODOLIST_IDS \
  proofhub-mcp:local
# or via docker-compose
docker compose up
```

Check health:

```bash
curl http://localhost:8080/health
# ok
```

Harness config (http):

```json
{
  "mcpServers": {
    "proofhub": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

Or if the harness requires docker for http:

```json
{
  "mcpServers": {
    "proofhub": {
      "command": "docker",
      "args": [
        "run", "-p", "8080:8080",
        "-e", "MCP_TRANSPORT=http",
        "-e", "MCP_HTTP_PORT=8080",
        "-e", "PROOFHUB_BASE_URL",
        "-e", "PROOFHUB_API_KEY",
        "-e", "PROOFHUB_USER_AGENT",
        "-e", "PROOFHUB_PROJECT_ID",
        "-e", "PROOFHUB_TODOLIST_IDS",
        "proofhub-mcp:local"
      ],
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

## Verify

```bash
go vet ./... && go test ./...
docker build -t proofhub-mcp:local .
# stdio
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | \
  PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy PROOFHUB_USER_AGENT="proofhub-mcp (test@example.com)" PROOFHUB_PROJECT_ID=1 PROOFHUB_TODOLIST_IDS=1 ./proofhub-mcp
# http
MCP_TRANSPORT=http MCP_HTTP_PORT=8080 PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy PROOFHUB_USER_AGENT="proofhub-mcp (test@example.com)" PROOFHUB_PROJECT_ID=1 PROOFHUB_TODOLIST_IDS=1 ./proofhub-mcp &
curl http://localhost:8080/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

## ProofHub API

- https://github.com/ProofHub/api_v3 (sections/tasks.md — tasks, subtasks, comments, history, copy, move)

## License

MIT
