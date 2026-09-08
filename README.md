# ProofHub MCP

ProofHub MCP — MCP server implementation of ProofHub API v3 (https://github.com/ProofHub/api_v3) for task management. Native MCP (stdio & Streamable HTTP), not a CLI wrapper. Scoped to a **single todolist** configured via ENV (`PROOFHUB_PROJECT_ID`/`PROOFHUB_TODOLIST_ID`) — cannot access other projects or lists. All tools automatically inject project/todolist from ENV.

## Environment

**Required:**
```bash
PROOFHUB_BASE_URL=https://yourcompany.proofhub.com
PROOFHUB_API_KEY=YOUR_API_KEY
PROOFHUB_USER_AGENT=proofhub-mcp (you@example.com)  # format AppName (email), ProofHub returns 400 without it
PROOFHUB_PROJECT_ID=8213786200                          # digits only, fail-fast if empty/invalid
PROOFHUB_TODOLIST_ID=271478716253                        # digits only, fail-fast if empty/invalid
```

**Optional (transport):**
```bash
MCP_TRANSPORT=stdio  # stdio (default) or http
MCP_HTTP_PORT=8080   # port for http, default 8080
```

ENV validation runs at startup — lowercase errors without leaking secrets (`proofhub_project_id is required and must be digits, got ""`).

### How to get your API key

1. Log in to your ProofHub account (`https://yourcompany.proofhub.com`).
2. Click your profile picture/avatar in the bottom-left corner.
3. Open **Profile** → **API Access** menu.
4. Copy the API key (used as `X-API-KEY` / `PROOFHUB_API_KEY`). Keep it secret — never commit it to git.

> Tip: On some ProofHub versions you need to click the profile picture 5 times to reveal the API key field.

## Tools (25) — single todolist scope

All tools are scoped to `PROOFHUB_PROJECT_ID`/`PROOFHUB_TODOLIST_ID` from ENV, except `label_*` and `people_list` which are global (no project/todolist). No `project_id`/`todolist_id` params in input — the agent only needs `task_id`, `subtask_id`, etc.

| Tool | Description | Input | Output |
|------|-------------|-------|--------|
| `task_list` | List all tasks in the scoped todolist (PROOFHUB_PROJECT_ID/PROOFHUB_TODOLIST_ID). Returns array of tasks with id, ticket, title, status. No project/list input required — injected from ENV. | - | `Task[]` JSON |
| `task_get` | Get a single task in the scoped todolist (ENV). Requires task_id (digits). Returns full details: title, description, dates, progress, stage, assignees, labels. | `task_id` (digits) | `Task` JSON |
| `task_create` | Create a new task in the scoped todolist. Requires title; optional description, start/due dates, estimates, assignees, labels. Project/todolist injected from ENV, cannot create outside scope. | `title` (req), `description`, `start_date`/`due_date` (YYYY-MM-DD), `estimated_hours`/`mins`, `assigned` (int[]), `labels` (int[]) | `Task` JSON |
| `task_update` | Update a task in the scoped todolist. Requires task_id; optional: title, description, dates (YYYY-MM-DD), estimated_hours/mins, logged_hours/mins, percent_progress 0-100, assignees, labels, completed (bool), stage_id. | `task_id` (req), `title`, `description`, `start_date`, `due_date`, `estimated_hours`/`mins`, `logged_hours`/`mins`, `percent_progress` 0-100, `assigned`, `labels`, `completed` (bool), `stage_id` | `Task` JSON |
| `task_delete` | Delete a task inside the scoped todolist. Can only delete within the ENV-configured todolist. | `task_id` | `deleted task <id>` text |
| `task_copy` | Copy (duplicate) a task inside the scoped todolist. Creates duplicate in the same todolist (single-todolist scope). Optional: new title, stage_id, copy_assignees/custom_fields/dates/comments. | `task_id` (req), `title`, `stage_id`, `copy_assignees`/`copy_custom_fields`/`copy_dates`/`copy_comments` (bool) | `Task` JSON |
| `task_move` | Move a task inside the scoped todolist (single-todolist scope). Cannot move to another project/list. Optional: title, stage_id, move_people, copy_assignees/custom_fields, move_dates, proof_comment, copy_comments, completed. | `task_id` (req), `title`, `stage_id`, `move_people`, `copy_assignees`/`custom_fields`, `move_dates`, `proof_comment`, `copy_comments`, `completed` | `Task` JSON |
| `subtask_list` | List subtasks under a task in the scoped todolist. Requires task_id. Returns array of subtasks. | `task_id` | `Subtask[]` JSON |
| `subtask_get` | Get a single subtask in the scoped todolist. Requires task_id and subtask_id. | `task_id`, `subtask_id` | `Subtask` JSON |
| `subtask_create` | Create a new subtask under a task in the scoped todolist. Requires task_id and title; optional: description, start/due (YYYY-MM-DD), estimated_hours/mins, assignees, labels. | `task_id`, `title` (req), `description`, `start_date`/`due_date`, `estimated_hours`/`mins`, `assigned`, `labels` | `Subtask` JSON |
| `subtask_update` | Update a subtask in the scoped todolist. Requires task_id and subtask_id; optional: title, description, dates, estimates, assignees, labels, completed. | `task_id`, `subtask_id`, `title`, `description`, `start_date`/`due_date`, `estimated_hours`/`mins`, `assigned`, `labels`, `completed` | `Subtask` JSON |
| `subtask_delete` | Delete a subtask in the scoped todolist. Requires task_id and subtask_id. Scoped to ENV todolist only. | `task_id`, `subtask_id` | text |
| `comment_list` | List comments on a task in the scoped todolist. Requires task_id. Returns array of comments with id, description, creator. | `task_id` | `Comment[]` JSON |
| `comment_get` | Get a single comment on a task in the scoped todolist. Requires task_id and comment_id. | `task_id`, `comment_id` | `Comment` JSON |
| `comment_create` | Create a comment on a task in the scoped todolist. Requires task_id and description (comment text). | `task_id`, `description` (req) | `Comment` JSON |
| `comment_update` | Update a comment on a task in the scoped todolist. Requires task_id, comment_id, and new description. | `task_id`, `comment_id`, `description` (req) | `Comment` JSON |
| `comment_delete` | Delete a comment on a task in the scoped todolist. Requires task_id and comment_id. | `task_id`, `comment_id` | text |
| `history_list` | List audit history for a task in the scoped todolist. Requires task_id. Returns activity history (updated, created, etc). | `task_id` | `TaskHistoryEntry[]` JSON |
| `history_get` | Get a single history entry for a task in the scoped todolist. Requires task_id and history_id. Returns change content. | `task_id`, `history_id` | `TaskHistoryDetail` JSON |
| `todolist_get` | Get the scoped todolist details (read-only). No input required — injected from PROOFHUB_PROJECT_ID/PROOFHUB_TODOLIST_ID. Returns title, privacy, archived, counts. | - | `Todolist` JSON |
| `label_list` | List all labels (global, not scoped to todolist). No input required. Returns id, name, color. | - | `Label[]` JSON |
| `label_get` | Get a single label by ID. Requires label_id (digits). Global lookup, not bound to ENV todolist. | `label_id` (digits) | `Label` JSON |
| `timesheet_list` | List timesheets in the scoped project (PROOFHUB_PROJECT_ID). No project input required — injected from ENV. Use to lookup timesheets before logging time. | - | `Timesheet[]` JSON |
| `timesheet_get` | Get a single timesheet in the scoped project. Requires timesheet_id (digits). Project injected from ENV. | `timesheet_id` (digits) | `Timesheet` JSON |
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

Lookup flow: `people_list` → find `id` for `first_name`/`email` → `task_create` with `assigned: [9526247227]`. `people_list` is global (not scoped to `PROOFHUB_PROJECT_ID`/`TODOLIST_ID`) — no input required, no filtering by project.

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
export PROOFHUB_TODOLIST_ID=271478716253
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
  -e PROOFHUB_TODOLIST_ID \
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
        "-e", "PROOFHUB_TODOLIST_ID",
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
  -e PROOFHUB_TODOLIST_ID \
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
        "-e", "PROOFHUB_TODOLIST_ID",
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
  PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy PROOFHUB_USER_AGENT="proofhub-mcp (test@example.com)" PROOFHUB_PROJECT_ID=1 PROOFHUB_TODOLIST_ID=1 ./proofhub-mcp
# http
MCP_TRANSPORT=http MCP_HTTP_PORT=8080 PROOFHUB_BASE_URL=https://example.com PROOFHUB_API_KEY=dummy PROOFHUB_USER_AGENT="proofhub-mcp (test@example.com)" PROOFHUB_PROJECT_ID=1 PROOFHUB_TODOLIST_ID=1 ./proofhub-mcp &
curl http://localhost:8080/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

## ProofHub API

- https://github.com/ProofHub/api_v3 (sections/tasks.md — tasks, subtasks, comments, history, copy, move)

## License

MIT
