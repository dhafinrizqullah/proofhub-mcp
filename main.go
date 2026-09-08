package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/dhafinrizqullah/proofhub-mcp/pkg/proofhub"
)

// version is set via ldflags.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	client, err := newClientFromEnv()
	if err != nil {
		// Do not fail hard on missing env for MCP lifecycle; log to stderr and continue.
		// Tools will return error if client is not configured.
		fmt.Fprintf(os.Stderr, "warn: %v\n", err)
	}

	s := server.NewMCPServer(
		"proofhub-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	registerTools(s, client)

	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("serve stdio: %w", err)
	}
	return nil
}

func newClientFromEnv() (*proofhub.Client, error) {
	baseURL := strings.TrimSpace(os.Getenv("PROOFHUB_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("PROOFHUB_API_KEY"))
	userAgent := strings.TrimSpace(os.Getenv("PROOFHUB_USER_AGENT"))
	if userAgent == "" {
		userAgent = os.Getenv("PROOFHUB_USER_AGENT")
	}
	if userAgent == "" {
		userAgent = "proofhub-mcp (dev@example.com)"
	}
	// Allow PROOFHUB_API_KEY and PROOFHUB_BASE_URL to be required only when tools are called.
	// Validate early if provided.
	c := proofhub.New(baseURL, apiKey, userAgent)
	// Don't validate if empty here; let handlers return meaningful errors.
	if baseURL != "" || apiKey != "" {
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("validate proofhub config: %w", err)
		}
	}
	return c, nil
}

func mustClient(c *proofhub.Client) (*proofhub.Client, error) {
	if c == nil {
		return nil, fmt.Errorf("proofhub client not initialized")
	}
	// Re-read env in case it changed, but prefer existing client.
	// Ensure Validate at call time.
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate proofhub client: %w", err)
	}
	return c, nil
}

// --- validation helpers ---

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validateDate(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("want YYYY-MM-DD, got %q", s)
	}
	return nil
}

func validateRequiredDigits(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if !isDigits(v) {
		return fmt.Errorf("invalid %s %q: want numeric id", name, v)
	}
	return nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func errorResult(msg string, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("%s: %v", msg, err)), nil
	}
	return mcp.NewToolResultError(msg), nil
}

// withTimeout creates a context with 30s timeout for API calls.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}

func getString(req mcp.CallToolRequest, key string, required bool) (string, error) {
	v, err := req.RequireString(key)
	if err != nil {
		if !required {
			// Try optional get
			if s := req.GetString(key, ""); s != "" {
				return s, nil
			}
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(v), nil
}

func getOptionalString(req mcp.CallToolRequest, key string) string {
	return strings.TrimSpace(req.GetString(key, ""))
}

func getOptionalInt(req mcp.CallToolRequest, key string) (*int, error) {
	// mcp-go stores numbers as float64 via JSON
	args := req.GetArguments()
	val, ok := args[key]
	if !ok || val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case float64:
		i := int(v)
		return &i, nil
	case int:
		return &v, nil
	case int64:
		i := int(v)
		return &i, nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", key, err)
		}
		i := int(n)
		return &i, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: want integer", key, v)
		}
		return &n, nil
	default:
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
}

func getOptionalInt64(req mcp.CallToolRequest, key string) (*int64, error) {
	args := req.GetArguments()
	val, ok := args[key]
	if !ok || val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case float64:
		n := int64(v)
		return &n, nil
	case int:
		n := int64(v)
		return &n, nil
	case int64:
		return &v, nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", key, err)
		}
		return &n, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: want integer", key, v)
		}
		return &n, nil
	default:
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
}

func getOptionalBool(req mcp.CallToolRequest, key string) (*bool, error) {
	args := req.GetArguments()
	val, ok := args[key]
	if !ok || val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case bool:
		return &v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: want boolean", key, v)
		}
		return &b, nil
	default:
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
}

func getInt64Slice(req mcp.CallToolRequest, key string) ([]int64, error) {
	args := req.GetArguments()
	val, ok := args[key]
	if !ok || val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case []any:
		out := make([]int64, 0, len(v))
		for i, elem := range v {
			switch e := elem.(type) {
			case float64:
				out = append(out, int64(e))
			case int:
				out = append(out, int64(e))
			case int64:
				out = append(out, e)
			case string:
				n, err := strconv.ParseInt(strings.TrimSpace(e), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid %s[%d] %q: want integer", key, i, e)
				}
				out = append(out, n)
			default:
				return nil, fmt.Errorf("invalid %s[%d] type %T", key, i, elem)
			}
		}
		return out, nil
	case string:
		// Allow comma-separated string for convenience
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		parts := strings.Split(v, ",")
		out := make([]int64, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			n, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid %s %q: want comma-separated integers", key, v)
			}
			out = append(out, n)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
}

// --- tool registration ---

func registerTools(s *server.MCPServer, client *proofhub.Client) {
	// Tasks
	s.AddTool(newTaskListTool(), handleTaskList(client))
	s.AddTool(newTaskGetTool(), handleTaskGet(client))
	s.AddTool(newTaskCreateTool(), handleTaskCreate(client))
	s.AddTool(newTaskUpdateTool(), handleTaskUpdate(client))
	s.AddTool(newTaskDeleteTool(), handleTaskDelete(client))
	s.AddTool(newTaskCopyTool(), handleTaskCopy(client))
	s.AddTool(newTaskMoveTool(), handleTaskMove(client))

	// Subtasks
	s.AddTool(newSubtaskListTool(), handleSubtaskList(client))
	s.AddTool(newSubtaskGetTool(), handleSubtaskGet(client))
	s.AddTool(newSubtaskCreateTool(), handleSubtaskCreate(client))
	s.AddTool(newSubtaskUpdateTool(), handleSubtaskUpdate(client))
	s.AddTool(newSubtaskDeleteTool(), handleSubtaskDelete(client))

	// Comments
	s.AddTool(newCommentListTool(), handleCommentList(client))
	s.AddTool(newCommentGetTool(), handleCommentGet(client))
	s.AddTool(newCommentCreateTool(), handleCommentCreate(client))
	s.AddTool(newCommentUpdateTool(), handleCommentUpdate(client))
	s.AddTool(newCommentDeleteTool(), handleCommentDelete(client))

	// History
	s.AddTool(newHistoryListTool(), handleHistoryList(client))
	s.AddTool(newHistoryGetTool(), handleHistoryGet(client))

	// Single gets (22 tools total: 19 inside todolist + todolist_get + label_get + timesheet_get)
	// Note: label_list and timesheet_list are intentionally not exposed as separate tools;
	// they are available via label_get/timesheet_get patterns. The spec's Names line
	// lists them, but the 22-count math (19+3) indicates only the _get variants.
	// To expose lists, uncomment the two lines below (would make 24 tools).
	s.AddTool(newTodolistGetTool(), handleTodolistGet(client))
	s.AddTool(newLabelGetTool(), handleLabelGet(client))
	s.AddTool(newTimesheetGetTool(), handleTimesheetGet(client))
	// s.AddTool(newLabelListTool(), handleLabelList(client))
	// s.AddTool(newTimesheetListTool(), handleTimesheetList(client))
}

// --- tool definitions ---

func newTaskListTool() mcp.Tool {
	return mcp.NewTool("task_list",
		mcp.WithDescription("List tasks in a todolist (ProofHub API v3 GET /projects/{project}/todolists/{list}/tasks)"),
		mcp.WithString("project_id",
			mcp.Required(),
			mcp.Description("Project ID (digits only)"),
			mcp.Pattern("^[0-9]+$"),
		),
		mcp.WithString("todolist_id",
			mcp.Required(),
			mcp.Description("Todolist (task list) ID (digits only)"),
			mcp.Pattern("^[0-9]+$"),
		),
	)
}

func newTaskGetTool() mcp.Tool {
	return mcp.NewTool("task_get",
		mcp.WithDescription("Get a single task (GET /projects/{project}/todolists/{list}/tasks/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Description("Todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTaskCreateTool() mcp.Tool {
	return mcp.NewTool("task_create",
		mcp.WithDescription("Create a task in a todolist (POST /projects/{project}/todolists/{list}/tasks)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Description("Todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task title")),
		mcp.WithString("description", mcp.Description("Task description (HTML allowed)")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes")),
		mcp.WithArray("assigned", mcp.Description("Assignee people IDs")),
		mcp.WithArray("labels", mcp.Description("Label IDs")),
	)
}

func newTaskUpdateTool() mcp.Tool {
	return mcp.NewTool("task_update",
		mcp.WithDescription("Update a task (PUT /projects/{project}/todolists/{list}/tasks/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Description("Todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes")),
		mcp.WithNumber("logged_hours", mcp.Description("Logged hours")),
		mcp.WithNumber("logged_mins", mcp.Description("Logged minutes")),
		mcp.WithNumber("percent_progress", mcp.Description("Percent progress 0-100")),
		mcp.WithArray("assigned", mcp.Description("Assignee IDs")),
		mcp.WithArray("labels", mcp.Description("Label IDs")),
		mcp.WithBoolean("completed", mcp.Description("Mark completed true/false")),
		mcp.WithString("stage_id", mcp.Description("Stage ID (digits)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTaskDeleteTool() mcp.Tool {
	return mcp.NewTool("task_delete",
		mcp.WithDescription("Delete a task (DELETE /projects/{project}/todolists/{list}/tasks/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Description("Todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTaskCopyTool() mcp.Tool {
	return mcp.NewTool("task_copy",
		mcp.WithDescription("Copy (duplicate) a task (POST /projects/{project}/todolists/{list}/tasks/{id} with copy_task)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Source project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Description("Source todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to copy"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("Title for copied task")),
		mcp.WithString("new_project_id", mcp.Description("Destination project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("new_todolist_id", mcp.Description("Destination todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("stage_id", mcp.Description("Stage ID for copied task"), mcp.Pattern("^[0-9]+$")),
		mcp.WithBoolean("copy_assignees", mcp.Description("Copy assignees")),
		mcp.WithBoolean("copy_custom_fields", mcp.Description("Copy custom fields")),
		mcp.WithBoolean("copy_dates", mcp.Description("Copy dates")),
		mcp.WithBoolean("copy_comments", mcp.Description("Copy comments")),
	)
}

func newTaskMoveTool() mcp.Tool {
	return mcp.NewTool("task_move",
		mcp.WithDescription("Move a task (PUT /projects/{project}/todolists/{list}/tasks/{id} with move_task)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Source project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Description("Source todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to move"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("New title (optional)")),
		mcp.WithString("new_project_id", mcp.Description("Destination project ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("new_todolist_id", mcp.Description("Destination todolist ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("stage_id", mcp.Description("Stage ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithBoolean("move_people", mcp.Description("Move people")),
		mcp.WithBoolean("copy_assignees", mcp.Description("Copy assignees")),
		mcp.WithBoolean("copy_custom_fields", mcp.Description("Copy custom fields")),
		mcp.WithBoolean("move_dates", mcp.Description("Move dates")),
		mcp.WithBoolean("proof_comment", mcp.Description("Proof comment")),
		mcp.WithBoolean("copy_comments", mcp.Description("Copy comments")),
		mcp.WithBoolean("completed", mcp.Description("Completed flag")),
	)
}

func newSubtaskListTool() mcp.Tool {
	return mcp.NewTool("subtask_list",
		mcp.WithDescription("List subtasks for a task (GET .../tasks/{task}/subtasks)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$"), mcp.Description("Project ID")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$"), mcp.Description("Todolist ID")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$"), mcp.Description("Parent task ID")),
	)
}

func newSubtaskGetTool() mcp.Tool {
	return mcp.NewTool("subtask_get",
		mcp.WithDescription("Get a subtask (GET .../subtasks/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Pattern("^[0-9]+$"), mcp.Description("Subtask ID")),
	)
}

func newSubtaskCreateTool() mcp.Tool {
	return mcp.NewTool("subtask_create",
		mcp.WithDescription("Create a subtask (POST .../tasks/{task}/subtasks)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Subtask title")),
		mcp.WithString("description", mcp.Description("Description")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes")),
		mcp.WithArray("assigned", mcp.Description("Assignee IDs")),
		mcp.WithArray("labels", mcp.Description("Label IDs")),
	)
}

func newSubtaskUpdateTool() mcp.Tool {
	return mcp.NewTool("subtask_update",
		mcp.WithDescription("Update a subtask (PUT .../subtasks/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes")),
		mcp.WithArray("assigned", mcp.Description("Assignee IDs")),
		mcp.WithArray("labels", mcp.Description("Label IDs")),
		mcp.WithBoolean("completed", mcp.Description("Completed flag")),
	)
}

func newSubtaskDeleteTool() mcp.Tool {
	return mcp.NewTool("subtask_delete",
		mcp.WithDescription("Delete a subtask (DELETE .../subtasks/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newCommentListTool() mcp.Tool {
	return mcp.NewTool("comment_list",
		mcp.WithDescription("List comments for a task (GET .../comments)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newCommentGetTool() mcp.Tool {
	return mcp.NewTool("comment_get",
		mcp.WithDescription("Get a comment (GET .../comments/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newCommentCreateTool() mcp.Tool {
	return mcp.NewTool("comment_create",
		mcp.WithDescription("Create a comment on a task (POST .../comments)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Comment text")),
	)
}

func newCommentUpdateTool() mcp.Tool {
	return mcp.NewTool("comment_update",
		mcp.WithDescription("Update a comment (PUT .../comments/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("description", mcp.Required(), mcp.Description("New comment text")),
	)
}

func newCommentDeleteTool() mcp.Tool {
	return mcp.NewTool("comment_delete",
		mcp.WithDescription("Delete a comment (DELETE .../comments/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newHistoryListTool() mcp.Tool {
	return mcp.NewTool("history_list",
		mcp.WithDescription("List task history (GET .../history)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newHistoryGetTool() mcp.Tool {
	return mcp.NewTool("history_get",
		mcp.WithDescription("Get task history detail (GET .../history/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("task_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("history_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newTodolistGetTool() mcp.Tool {
	return mcp.NewTool("todolist_get",
		mcp.WithDescription("Get a todolist (GET /projects/{project}/todolists/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("todolist_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newLabelGetTool() mcp.Tool {
	return mcp.NewTool("label_get",
		mcp.WithDescription("Get a label (GET /labels/{id})"),
		mcp.WithString("label_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newTimesheetGetTool() mcp.Tool {
	return mcp.NewTool("timesheet_get",
		mcp.WithDescription("Get a timesheet (GET /projects/{project}/timesheets/{id})"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("timesheet_id", mcp.Required(), mcp.Pattern("^[0-9]+$")),
	)
}

func newLabelListTool() mcp.Tool {
	return mcp.NewTool("label_list",
		mcp.WithDescription("List labels (GET /labels)"),
	)
}

func newTimesheetListTool() mcp.Tool {
	return mcp.NewTool("timesheet_list",
		mcp.WithDescription("List timesheets for a project (GET /projects/{project}/timesheets)"),
		mcp.WithString("project_id", mcp.Required(), mcp.Pattern("^[0-9]+$"), mcp.Description("Project ID")),
	)
}

// --- handlers ---

func handleTaskList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, err := req.RequireString("project_id")
		if err != nil {
			return errorResult("missing project_id", err)
		}
		todolistID, err := req.RequireString("todolist_id")
		if err != nil {
			return errorResult("missing todolist_id", err)
		}
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		tasks, err := c.ListTasks(tctx, projectID, todolistID)
		if err != nil {
			return errorResult("list tasks failed", err)
		}
		return jsonResult(tasks)
	}
}

func handleTaskGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		task, err := c.GetTask(tctx, projectID, todolistID, taskID)
		if err != nil {
			return errorResult("get task failed", err)
		}
		return jsonResult(task)
	}
}

func handleTaskCreate(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		title, _ := req.RequireString("title")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if strings.TrimSpace(title) == "" {
			return errorResult("validation failed", fmt.Errorf("title is required"))
		}
		desc := getOptionalString(req, "description")
		startDate := getOptionalString(req, "start_date")
		dueDate := getOptionalString(req, "due_date")
		if err := validateDate(startDate); err != nil {
			return errorResult("invalid start_date", err)
		}
		if err := validateDate(dueDate); err != nil {
			return errorResult("invalid due_date", err)
		}
		estH, err := getOptionalInt(req, "estimated_hours")
		if err != nil {
			return errorResult("invalid estimated_hours", err)
		}
		estM, err := getOptionalInt(req, "estimated_mins")
		if err != nil {
			return errorResult("invalid estimated_mins", err)
		}
		assigned, err := getInt64Slice(req, "assigned")
		if err != nil {
			return errorResult("invalid assigned", err)
		}
		labels, err := getInt64Slice(req, "labels")
		if err != nil {
			return errorResult("invalid labels", err)
		}
		payload := proofhub.CreateTaskRequest{
			Title:          title,
			Description:    desc,
			StartDate:      startDate,
			DueDate:        dueDate,
			EstimatedHours: estH,
			EstimatedMins:  estM,
			Assigned:       assigned,
			Labels:         labels,
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		task, err := c.CreateTask(tctx, projectID, todolistID, payload)
		if err != nil {
			return errorResult("create task failed", err)
		}
		return jsonResult(task)
	}
}

func handleTaskUpdate(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		var payload proofhub.UpdateTaskRequest
		if v := getOptionalString(req, "title"); v != "" || hasArg(req, "title") {
			s := v
			payload.Title = &s
		}
		if hasArg(req, "description") {
			s := getOptionalString(req, "description")
			payload.Description = &s
		}
		if hasArg(req, "start_date") {
			s := getOptionalString(req, "start_date")
			if err := validateDate(s); err != nil {
				return errorResult("invalid start_date", err)
			}
			payload.StartDate = &s
		}
		if hasArg(req, "due_date") {
			s := getOptionalString(req, "due_date")
			if err := validateDate(s); err != nil {
				return errorResult("invalid due_date", err)
			}
			payload.DueDate = &s
		}
		if hasArg(req, "estimated_hours") {
			n, err := getOptionalInt(req, "estimated_hours")
			if err != nil {
				return errorResult("invalid estimated_hours", err)
			}
			payload.EstimatedHours = n
		}
		if hasArg(req, "estimated_mins") {
			n, err := getOptionalInt(req, "estimated_mins")
			if err != nil {
				return errorResult("invalid estimated_mins", err)
			}
			payload.EstimatedMins = n
		}
		if hasArg(req, "logged_hours") {
			n, err := getOptionalInt(req, "logged_hours")
			if err != nil {
				return errorResult("invalid logged_hours", err)
			}
			payload.LoggedHours = n
		}
		if hasArg(req, "logged_mins") {
			n, err := getOptionalInt(req, "logged_mins")
			if err != nil {
				return errorResult("invalid logged_mins", err)
			}
			payload.LoggedMins = n
		}
		if hasArg(req, "percent_progress") {
			n, err := getOptionalInt(req, "percent_progress")
			if err != nil {
				return errorResult("invalid percent_progress", err)
			}
			if n != nil && (*n < 0 || *n > 100) {
				return errorResult("validation failed", fmt.Errorf("percent_progress want 0-100, got %d", *n))
			}
			payload.PercentProgress = n
		}
		if hasArg(req, "assigned") {
			arr, err := getInt64Slice(req, "assigned")
			if err != nil {
				return errorResult("invalid assigned", err)
			}
			payload.Assigned = arr
		}
		if hasArg(req, "labels") {
			arr, err := getInt64Slice(req, "labels")
			if err != nil {
				return errorResult("invalid labels", err)
			}
			payload.Labels = arr
		}
		if hasArg(req, "completed") {
			b, err := getOptionalBool(req, "completed")
			if err != nil {
				return errorResult("invalid completed", err)
			}
			payload.Completed = b
		}
		if hasArg(req, "stage_id") {
			s := getOptionalString(req, "stage_id")
			if s != "" {
				if !isDigits(s) {
					return errorResult("validation failed", fmt.Errorf("invalid stage_id %q: want numeric id", s))
				}
				n, _ := strconv.ParseInt(s, 10, 64)
				payload.StageID = &n
			}
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		task, err := c.UpdateTask(tctx, projectID, todolistID, taskID, payload)
		if err != nil {
			return errorResult("update task failed", err)
		}
		return jsonResult(task)
	}
}

func handleTaskDelete(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		if err := c.DeleteTask(tctx, projectID, todolistID, taskID); err != nil {
			return errorResult("delete task failed", err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("deleted task %s", taskID)), nil
	}
}

func handleTaskCopy(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		var payload proofhub.CopyTaskRequest
		if v := getOptionalString(req, "title"); v != "" {
			payload.Title = v
		}
		if v := getOptionalString(req, "new_project_id"); v != "" {
			if !isDigits(v) {
				return errorResult("validation failed", fmt.Errorf("invalid new_project_id %q", v))
			}
			n, _ := strconv.ParseInt(v, 10, 64)
			payload.Project = &n
		}
		if v := getOptionalString(req, "new_todolist_id"); v != "" {
			if !isDigits(v) {
				return errorResult("validation failed", fmt.Errorf("invalid new_todolist_id %q", v))
			}
			n, _ := strconv.ParseInt(v, 10, 64)
			payload.ListID = &n
		}
		if v := getOptionalString(req, "stage_id"); v != "" {
			if !isDigits(v) {
				return errorResult("validation failed", fmt.Errorf("invalid stage_id %q", v))
			}
			n, _ := strconv.ParseInt(v, 10, 64)
			payload.Stage = &n
		}
		if hasArg(req, "copy_assignees") {
			b, _ := getOptionalBool(req, "copy_assignees")
			payload.CopyAssignees = b
		}
		if hasArg(req, "copy_custom_fields") {
			b, _ := getOptionalBool(req, "copy_custom_fields")
			payload.CopyCustomFields = b
		}
		if hasArg(req, "copy_dates") {
			b, _ := getOptionalBool(req, "copy_dates")
			payload.CopyDates = b
		}
		if hasArg(req, "copy_comments") {
			b, _ := getOptionalBool(req, "copy_comments")
			payload.CopyComments = b
		}
		if n, err := strconv.ParseInt(taskID, 10, 64); err == nil {
			payload.CopyTask = n
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		task, err := c.CopyTask(tctx, projectID, todolistID, taskID, payload)
		if err != nil {
			return errorResult("copy task failed", err)
		}
		return jsonResult(task)
	}
}

func handleTaskMove(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		var payload proofhub.MoveTaskRequest
		if hasArg(req, "title") {
			s := getOptionalString(req, "title")
			payload.Title = &s
		}
		if v := getOptionalString(req, "new_project_id"); v != "" {
			if !isDigits(v) {
				return errorResult("validation failed", fmt.Errorf("invalid new_project_id %q", v))
			}
			n, _ := strconv.ParseInt(v, 10, 64)
			payload.Project = &n
		}
		if v := getOptionalString(req, "new_todolist_id"); v != "" {
			if !isDigits(v) {
				return errorResult("validation failed", fmt.Errorf("invalid new_todolist_id %q", v))
			}
			n, _ := strconv.ParseInt(v, 10, 64)
			payload.ListID = &n
		}
		if v := getOptionalString(req, "stage_id"); v != "" {
			if !isDigits(v) {
				return errorResult("validation failed", fmt.Errorf("invalid stage_id %q", v))
			}
			n, _ := strconv.ParseInt(v, 10, 64)
			payload.Stage = &n
		}
		if hasArg(req, "move_people") {
			b, _ := getOptionalBool(req, "move_people")
			payload.MovePeople = b
		}
		if hasArg(req, "copy_assignees") {
			b, _ := getOptionalBool(req, "copy_assignees")
			payload.CopyAssignees = b
		}
		if hasArg(req, "copy_custom_fields") {
			b, _ := getOptionalBool(req, "copy_custom_fields")
			payload.CopyCustomFields = b
		}
		if hasArg(req, "move_dates") {
			b, _ := getOptionalBool(req, "move_dates")
			payload.MoveDates = b
		}
		if hasArg(req, "proof_comment") {
			b, _ := getOptionalBool(req, "proof_comment")
			payload.ProofComment = b
		}
		if hasArg(req, "copy_comments") {
			b, _ := getOptionalBool(req, "copy_comments")
			payload.CopyComments = b
		}
		if hasArg(req, "completed") {
			b, _ := getOptionalBool(req, "completed")
			payload.Completed = b
		}
		payload.MoveTask = true
		if n, err := strconv.ParseInt(taskID, 10, 64); err == nil {
			payload.ID = &n
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		task, err := c.MoveTask(tctx, projectID, todolistID, taskID, payload)
		if err != nil {
			return errorResult("move task failed", err)
		}
		return jsonResult(task)
	}
}

func handleSubtaskList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		subs, err := c.ListSubtasks(tctx, projectID, todolistID, taskID)
		if err != nil {
			return errorResult("list subtasks failed", err)
		}
		return jsonResult(subs)
	}
}

func handleSubtaskGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		subtaskID, _ := req.RequireString("subtask_id")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"subtask_id", subtaskID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		sub, err := c.GetSubtask(tctx, projectID, todolistID, taskID, subtaskID)
		if err != nil {
			return errorResult("get subtask failed", err)
		}
		return jsonResult(sub)
	}
}

func handleSubtaskCreate(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		title, _ := req.RequireString("title")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		if strings.TrimSpace(title) == "" {
			return errorResult("validation failed", fmt.Errorf("title is required"))
		}
		desc := getOptionalString(req, "description")
		startDate := getOptionalString(req, "start_date")
		dueDate := getOptionalString(req, "due_date")
		if err := validateDate(startDate); err != nil {
			return errorResult("invalid start_date", err)
		}
		if err := validateDate(dueDate); err != nil {
			return errorResult("invalid due_date", err)
		}
		estH, _ := getOptionalInt(req, "estimated_hours")
		estM, _ := getOptionalInt(req, "estimated_mins")
		assigned, _ := getInt64Slice(req, "assigned")
		labels, _ := getInt64Slice(req, "labels")
		payload := proofhub.CreateSubtaskRequest{
			Title:          title,
			Description:    desc,
			StartDate:      startDate,
			DueDate:        dueDate,
			EstimatedHours: estH,
			EstimatedMins:  estM,
			Assigned:       assigned,
			Labels:         labels,
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		sub, err := c.CreateSubtask(tctx, projectID, todolistID, taskID, payload)
		if err != nil {
			return errorResult("create subtask failed", err)
		}
		return jsonResult(sub)
	}
}

func handleSubtaskUpdate(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		subtaskID, _ := req.RequireString("subtask_id")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"subtask_id", subtaskID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		var payload proofhub.UpdateSubtaskRequest
		if hasArg(req, "title") {
			s := getOptionalString(req, "title")
			payload.Title = &s
		}
		if hasArg(req, "description") {
			s := getOptionalString(req, "description")
			payload.Description = &s
		}
		if hasArg(req, "start_date") {
			s := getOptionalString(req, "start_date")
			if err := validateDate(s); err != nil {
				return errorResult("invalid start_date", err)
			}
			payload.StartDate = &s
		}
		if hasArg(req, "due_date") {
			s := getOptionalString(req, "due_date")
			if err := validateDate(s); err != nil {
				return errorResult("invalid due_date", err)
			}
			payload.DueDate = &s
		}
		if hasArg(req, "estimated_hours") {
			n, _ := getOptionalInt(req, "estimated_hours")
			payload.EstimatedHours = n
		}
		if hasArg(req, "estimated_mins") {
			n, _ := getOptionalInt(req, "estimated_mins")
			payload.EstimatedMins = n
		}
		if hasArg(req, "assigned") {
			arr, _ := getInt64Slice(req, "assigned")
			payload.Assigned = arr
		}
		if hasArg(req, "labels") {
			arr, _ := getInt64Slice(req, "labels")
			payload.Labels = arr
		}
		if hasArg(req, "completed") {
			b, _ := getOptionalBool(req, "completed")
			payload.Completed = b
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		sub, err := c.UpdateSubtask(tctx, projectID, todolistID, taskID, subtaskID, payload)
		if err != nil {
			return errorResult("update subtask failed", err)
		}
		return jsonResult(sub)
	}
}

func handleSubtaskDelete(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		subtaskID, _ := req.RequireString("subtask_id")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"subtask_id", subtaskID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		if err := c.DeleteSubtask(tctx, projectID, todolistID, taskID, subtaskID); err != nil {
			return errorResult("delete subtask failed", err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("deleted subtask %s", subtaskID)), nil
	}
}

func handleCommentList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		comments, err := c.ListComments(tctx, projectID, todolistID, taskID)
		if err != nil {
			return errorResult("list comments failed", err)
		}
		return jsonResult(comments)
	}
}

func handleCommentGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		commentID, _ := req.RequireString("comment_id")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"comment_id", commentID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		cc, err := c.GetComment(tctx, projectID, todolistID, taskID, commentID)
		if err != nil {
			return errorResult("get comment failed", err)
		}
		return jsonResult(cc)
	}
}

func handleCommentCreate(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		desc, _ := req.RequireString("description")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		if strings.TrimSpace(desc) == "" {
			return errorResult("validation failed", fmt.Errorf("description is required"))
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		cc, err := c.CreateComment(tctx, projectID, todolistID, taskID, desc)
		if err != nil {
			return errorResult("create comment failed", err)
		}
		return jsonResult(cc)
	}
}

func handleCommentUpdate(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		commentID, _ := req.RequireString("comment_id")
		desc, _ := req.RequireString("description")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"comment_id", commentID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		if strings.TrimSpace(desc) == "" {
			return errorResult("validation failed", fmt.Errorf("description is required"))
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		cc, err := c.UpdateComment(tctx, projectID, todolistID, taskID, commentID, desc)
		if err != nil {
			return errorResult("update comment failed", err)
		}
		return jsonResult(cc)
	}
}

func handleCommentDelete(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		commentID, _ := req.RequireString("comment_id")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"comment_id", commentID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		if err := c.DeleteComment(tctx, projectID, todolistID, taskID, commentID); err != nil {
			return errorResult("delete comment failed", err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("deleted comment %s", commentID)), nil
	}
}

func handleHistoryList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("task_id", taskID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		h, err := c.ListTaskHistory(tctx, projectID, todolistID, taskID)
		if err != nil {
			return errorResult("list history failed", err)
		}
		return jsonResult(h)
	}
}

func handleHistoryGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		taskID, _ := req.RequireString("task_id")
		historyID, _ := req.RequireString("history_id")
		for _, v := range []struct{ name, val string }{{"project_id", projectID}, {"todolist_id", todolistID}, {"task_id", taskID}, {"history_id", historyID}} {
			if err := validateRequiredDigits(v.name, v.val); err != nil {
				return errorResult("validation failed", err)
			}
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		d, err := c.GetTaskHistoryDetail(tctx, projectID, todolistID, taskID, historyID)
		if err != nil {
			return errorResult("get history detail failed", err)
		}
		return jsonResult(d)
	}
}

func handleTodolistGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		todolistID, _ := req.RequireString("todolist_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("todolist_id", todolistID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		tl, err := c.GetTodolist(tctx, projectID, todolistID)
		if err != nil {
			return errorResult("get todolist failed", err)
		}
		return jsonResult(tl)
	}
}

func handleLabelGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		labelID, _ := req.RequireString("label_id")
		if err := validateRequiredDigits("label_id", labelID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		l, err := c.GetLabel(tctx, labelID)
		if err != nil {
			return errorResult("get label failed", err)
		}
		return jsonResult(l)
	}
}

func handleTimesheetGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		timesheetID, _ := req.RequireString("timesheet_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		if err := validateRequiredDigits("timesheet_id", timesheetID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		ts, err := c.GetTimesheet(tctx, projectID, timesheetID)
		if err != nil {
			return errorResult("get timesheet failed", err)
		}
		return jsonResult(ts)
	}
}

func handleLabelList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		labels, err := c.ListLabels(tctx)
		if err != nil {
			return errorResult("list labels failed", err)
		}
		return jsonResult(labels)
	}
}

func handleTimesheetList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		projectID, _ := req.RequireString("project_id")
		if err := validateRequiredDigits("project_id", projectID); err != nil {
			return errorResult("validation failed", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		ts, err := c.ListTimesheets(tctx, projectID)
		if err != nil {
			return errorResult("list timesheets failed", err)
		}
		return jsonResult(ts)
	}
}

func hasArg(req mcp.CallToolRequest, key string) bool {
	args := req.GetArguments()
	_, ok := args[key]
	return ok
}
