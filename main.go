package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/dhafinrizqullah/proofhub-mcp/pkg/proofhub"
)

var version = "dev"

type config struct {
	baseURL    string
	apiKey     string
	userAgent  string
	projectID  string
	todolistID string
	transport  string
	httpPort   string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := loadConfig(args)
	if err != nil {
		return err
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	client := proofhub.New(cfg.baseURL, cfg.apiKey, cfg.userAgent)
	if err := client.Validate(); err != nil {
		return fmt.Errorf("validate proofhub config: %w", err)
	}

	s := server.NewMCPServer(
		"proofhub-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	registerTools(s, client, cfg.projectID, cfg.todolistID)

	switch cfg.transport {
	case "http":
		return serveHTTP(s, cfg.httpPort)
	default:
		if err := server.ServeStdio(s); err != nil {
			return fmt.Errorf("serve stdio: %w", err)
		}
		return nil
	}
}

func loadConfig(args []string) (*config, error) {
	cfg := &config{
		baseURL:    strings.TrimSpace(os.Getenv("PROOFHUB_BASE_URL")),
		apiKey:     strings.TrimSpace(os.Getenv("PROOFHUB_API_KEY")),
		userAgent:  strings.TrimSpace(os.Getenv("PROOFHUB_USER_AGENT")),
		projectID:  strings.TrimSpace(os.Getenv("PROOFHUB_PROJECT_ID")),
		todolistID: strings.TrimSpace(os.Getenv("PROOFHUB_TODOLIST_ID")),
		transport:  strings.TrimSpace(os.Getenv("MCP_TRANSPORT")),
		httpPort:   strings.TrimSpace(os.Getenv("MCP_HTTP_PORT")),
	}
	if cfg.transport == "" {
		cfg.transport = "stdio"
	}
	if cfg.httpPort == "" {
		cfg.httpPort = "8080"
	}
	if cfg.userAgent == "" {
		cfg.userAgent = "proofhub-mcp (dev@example.com)"
	}

	fs := flag.NewFlagSet("proofhub-mcp", flag.ContinueOnError)
	fs.StringVar(&cfg.transport, "transport", cfg.transport, "transport: stdio or http")
	fs.StringVar(&cfg.httpPort, "http-port", cfg.httpPort, "http port when transport=http")
	// Also allow --transport and --http-port via args, but also support env already.
	// Parse only known flags, ignore unknown to allow MCP inspector args.
	// Use flag.NFlag to detect if provided.
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	// Normalize transport
	cfg.transport = strings.ToLower(strings.TrimSpace(cfg.transport))
	if cfg.transport != "stdio" && cfg.transport != "http" {
		return nil, fmt.Errorf("invalid mcp_transport %q: want stdio or http", cfg.transport)
	}
	if cfg.httpPort != "" {
		if _, err := strconv.Atoi(cfg.httpPort); err != nil {
			return nil, fmt.Errorf("invalid mcp_http_port %q: want numeric port", cfg.httpPort)
		}
	}
	// Re-read env for project/todolist if flags didn't override (flags don't cover them)
	// Allow --project-id and --todolist-id as flags for local testing
	var flagProject, flagTodolist string
	fs2 := flag.NewFlagSet("extra", flag.ContinueOnError)
	fs2.StringVar(&flagProject, "project-id", "", "")
	fs2.StringVar(&flagTodolist, "todolist-id", "", "")
	_ = fs2.Parse(args)
	if flagProject != "" {
		cfg.projectID = strings.TrimSpace(flagProject)
	}
	if flagTodolist != "" {
		cfg.todolistID = strings.TrimSpace(flagTodolist)
	}
	return cfg, nil
}

func validateConfig(cfg *config) error {
	if strings.TrimSpace(cfg.baseURL) == "" {
		return fmt.Errorf("proofhub_base_url is required")
	}
	if strings.TrimSpace(cfg.apiKey) == "" {
		return fmt.Errorf("proofhub_api_key is required")
	}
	if !isDigits(cfg.projectID) {
		return fmt.Errorf("proofhub_project_id is required and must be digits, got %q", cfg.projectID)
	}
	if !isDigits(cfg.todolistID) {
		return fmt.Errorf("proofhub_todolist_id is required and must be digits, got %q", cfg.todolistID)
	}
	// User-Agent validation is done via proofhub.Client.Validate, but also check format here for fail fast
	if cfg.userAgent != "" {
		// Use same pattern as client
		if !strings.Contains(cfg.userAgent, " (") || !strings.Contains(cfg.userAgent, "@") {
			return fmt.Errorf("user-agent must look like %q, got %q", "AppName (name@example.com)", cfg.userAgent)
		}
	}
	return nil
}

func serveHTTP(s *server.MCPServer, port string) error {
	httpServer := server.NewStreamableHTTPServer(s)
	addr := ":" + port
	fmt.Fprintf(os.Stderr, "proofhub-mcp http listening on %s/mcp (transport=http)\n", addr)
	// Use custom mux to ensure /mcp endpoint is correct
	mux := http.NewServeMux()
	mux.Handle("/mcp", httpServer)
	// Also handle root for health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http serve: %w", err)
	}
	return nil
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

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}

func getOptionalString(req mcp.CallToolRequest, key string) string {
	return strings.TrimSpace(req.GetString(key, ""))
}

func getOptionalInt(req mcp.CallToolRequest, key string) (*int, error) {
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

func hasArg(req mcp.CallToolRequest, key string) bool {
	args := req.GetArguments()
	_, ok := args[key]
	return ok
}

func mustClient(c *proofhub.Client) (*proofhub.Client, error) {
	if c == nil {
		return nil, fmt.Errorf("proofhub client not initialized")
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate proofhub client: %w", err)
	}
	return c, nil
}

// --- tool registration ---
// Scope: single todolist via ENV PROOFHUB_PROJECT_ID / PROOFHUB_TODOLIST_ID PROOFHUB_PROJECT_ID / PROOFHUB_TODOLIST_ID

func registerTools(s *server.MCPServer, client *proofhub.Client, projectID, todolistID string) {
	s.AddTool(newTaskListTool(), handleTaskList(client, projectID, todolistID))
	s.AddTool(newTaskGetTool(), handleTaskGet(client, projectID, todolistID))
	s.AddTool(newTaskCreateTool(), handleTaskCreate(client, projectID, todolistID))
	s.AddTool(newTaskUpdateTool(), handleTaskUpdate(client, projectID, todolistID))
	s.AddTool(newTaskDeleteTool(), handleTaskDelete(client, projectID, todolistID))
	s.AddTool(newTaskCopyTool(), handleTaskCopy(client, projectID, todolistID))
	s.AddTool(newTaskMoveTool(), handleTaskMove(client, projectID, todolistID))

	s.AddTool(newSubtaskListTool(), handleSubtaskList(client, projectID, todolistID))
	s.AddTool(newSubtaskGetTool(), handleSubtaskGet(client, projectID, todolistID))
	s.AddTool(newSubtaskCreateTool(), handleSubtaskCreate(client, projectID, todolistID))
	s.AddTool(newSubtaskUpdateTool(), handleSubtaskUpdate(client, projectID, todolistID))
	s.AddTool(newSubtaskDeleteTool(), handleSubtaskDelete(client, projectID, todolistID))

	s.AddTool(newCommentListTool(), handleCommentList(client, projectID, todolistID))
	s.AddTool(newCommentGetTool(), handleCommentGet(client, projectID, todolistID))
	s.AddTool(newCommentCreateTool(), handleCommentCreate(client, projectID, todolistID))
	s.AddTool(newCommentUpdateTool(), handleCommentUpdate(client, projectID, todolistID))
	s.AddTool(newCommentDeleteTool(), handleCommentDelete(client, projectID, todolistID))

	s.AddTool(newHistoryListTool(), handleHistoryList(client, projectID, todolistID))
	s.AddTool(newHistoryGetTool(), handleHistoryGet(client, projectID, todolistID))

	s.AddTool(newTodolistGetTool(), handleTodolistGet(client, projectID, todolistID))
	s.AddTool(newLabelListTool(), handleLabelList(client))
	s.AddTool(newLabelGetTool(), handleLabelGet(client))
	s.AddTool(newTimesheetListTool(), handleTimesheetList(client, projectID))
	s.AddTool(newTimesheetGetTool(), handleTimesheetGet(client, projectID))
	s.AddTool(newPeopleListTool(), handlePeopleList(client))
}

// --- tool definitions with full descriptions ---

func newTaskListTool() mcp.Tool {
	return mcp.NewTool("task_list",
		mcp.WithDescription("List all tasks in the scoped todolist (PROOFHUB_PROJECT_ID/PROOFHUB_TODOLIST_ID). Returns array of tasks with id, ticket, title, status. No project/list input required — injected from ENV."),
	)
}

func newTaskGetTool() mcp.Tool {
	return mcp.NewTool("task_get",
		mcp.WithDescription("Get a single task in the scoped todolist (ENV). Requires task_id (digits). Returns full details: title, description, dates, progress, stage, assignees, labels."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID (digits only)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTaskCreateTool() mcp.Tool {
	return mcp.NewTool("task_create",
		mcp.WithDescription("Create a new task in the scoped todolist. Requires title; optional description, start/due dates, estimates, assignees, labels. Project/todolist injected from ENV, cannot create outside scope."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task title (required)")),
		mcp.WithString("description", mcp.Description("Task description (HTML allowed, e.g. <b>Label:</b> ...<br>)")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours (integer)")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes (integer)")),
		mcp.WithArray("assigned", mcp.Description("Assignee people IDs (array of integers, e.g. [9526247227])")),
		mcp.WithArray("labels", mcp.Description("Label IDs (array of integers, e.g. [5775379693,7273284338])")),
	)
}

func newTaskUpdateTool() mcp.Tool {
	return mcp.NewTool("task_update",
		mcp.WithDescription("Update a task in the scoped todolist. Requires task_id; optional: title, description, dates (YYYY-MM-DD), estimated_hours/mins, logged_hours/mins, percent_progress 0-100, assignees, labels, completed (bool), stage_id."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID (digits)"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description (HTML allowed)")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes")),
		mcp.WithNumber("logged_hours", mcp.Description("Logged hours")),
		mcp.WithNumber("logged_mins", mcp.Description("Logged minutes")),
		mcp.WithNumber("percent_progress", mcp.Description("Percent progress 0-100")),
		mcp.WithArray("assigned", mcp.Description("Assignee IDs (array)")),
		mcp.WithArray("labels", mcp.Description("Label IDs (array)")),
		mcp.WithBoolean("completed", mcp.Description("Mark completed true/false")),
		mcp.WithString("stage_id", mcp.Description("Stage ID (digits)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTaskDeleteTool() mcp.Tool {
	return mcp.NewTool("task_delete",
		mcp.WithDescription("Delete a task inside the scoped todolist. Can only delete within the ENV-configured todolist."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID (digits)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTaskCopyTool() mcp.Tool {
	return mcp.NewTool("task_copy",
		mcp.WithDescription("Copy (duplicate) a task inside the scoped todolist. Creates duplicate in the same todolist (single-todolist scope). Optional: new title, stage_id, copy_assignees/custom_fields/dates/comments."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to copy (digits)"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("Title for copied task (optional, default 'Copy of ...')")),
		mcp.WithString("stage_id", mcp.Description("Stage ID for copied task (digits)"), mcp.Pattern("^[0-9]+$")),
		mcp.WithBoolean("copy_assignees", mcp.Description("Copy assignees (default true)")),
		mcp.WithBoolean("copy_custom_fields", mcp.Description("Copy custom fields (default true)")),
		mcp.WithBoolean("copy_dates", mcp.Description("Copy dates (default true)")),
		mcp.WithBoolean("copy_comments", mcp.Description("Copy comments (default true)")),
	)
}

func newTaskMoveTool() mcp.Tool {
	return mcp.NewTool("task_move",
		mcp.WithDescription("Move a task inside the scoped todolist (single-todolist scope). Cannot move to another project/list. Optional: title, stage_id, move_people, copy_assignees/custom_fields, move_dates, proof_comment, copy_comments, completed."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to move (digits)"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("New title (optional)")),
		mcp.WithString("stage_id", mcp.Description("Stage ID (digits)"), mcp.Pattern("^[0-9]+$")),
		mcp.WithBoolean("move_people", mcp.Description("Move people (default true)")),
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
		mcp.WithDescription("List subtasks under a task in the scoped todolist. Requires task_id. Returns array of subtasks."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Parent task ID (digits)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newSubtaskGetTool() mcp.Tool {
	return mcp.NewTool("subtask_get",
		mcp.WithDescription("Get a single subtask in the scoped todolist. Requires task_id and subtask_id."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Parent task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Description("Subtask ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newSubtaskCreateTool() mcp.Tool {
	return mcp.NewTool("subtask_create",
		mcp.WithDescription("Create a new subtask under a task in the scoped todolist. Requires task_id and title; optional: description, start/due (YYYY-MM-DD), estimated_hours/mins, assignees, labels."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Parent task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Subtask title (required)")),
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
		mcp.WithDescription("Update a subtask in the scoped todolist. Requires task_id and subtask_id; optional: title, description, dates, estimates, assignees, labels, completed."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Parent task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Description("Subtask ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("start_date", mcp.Description("Start date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithString("due_date", mcp.Description("Due date YYYY-MM-DD"), mcp.Pattern("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")),
		mcp.WithNumber("estimated_hours", mcp.Description("Estimated hours")),
		mcp.WithNumber("estimated_mins", mcp.Description("Estimated minutes")),
		mcp.WithArray("assigned", mcp.Description("Assignee IDs")),
		mcp.WithArray("labels", mcp.Description("Label IDs")),
		mcp.WithBoolean("completed", mcp.Description("Completed flag true/false")),
	)
}

func newSubtaskDeleteTool() mcp.Tool {
	return mcp.NewTool("subtask_delete",
		mcp.WithDescription("Delete a subtask in the scoped todolist. Requires task_id and subtask_id. Scoped to ENV todolist only."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Parent task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Description("Subtask ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newCommentListTool() mcp.Tool {
	return mcp.NewTool("comment_list",
		mcp.WithDescription("List comments on a task in the scoped todolist. Requires task_id. Returns array of comments with id, description, creator."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newCommentGetTool() mcp.Tool {
	return mcp.NewTool("comment_get",
		mcp.WithDescription("Get a single comment on a task in the scoped todolist. Requires task_id and comment_id."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newCommentCreateTool() mcp.Tool {
	return mcp.NewTool("comment_create",
		mcp.WithDescription("Create a comment on a task in the scoped todolist. Requires task_id and description (comment text)."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Comment text (required)")),
	)
}

func newCommentUpdateTool() mcp.Tool {
	return mcp.NewTool("comment_update",
		mcp.WithDescription("Update a comment on a task in the scoped todolist. Requires task_id, comment_id, and new description."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("description", mcp.Required(), mcp.Description("New comment text (required)")),
	)
}

func newCommentDeleteTool() mcp.Tool {
	return mcp.NewTool("comment_delete",
		mcp.WithDescription("Delete a comment on a task in the scoped todolist. Requires task_id and comment_id."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newHistoryListTool() mcp.Tool {
	return mcp.NewTool("history_list",
		mcp.WithDescription("List audit history for a task in the scoped todolist. Requires task_id. Returns activity history (updated, created, etc)."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newHistoryGetTool() mcp.Tool {
	return mcp.NewTool("history_get",
		mcp.WithDescription("Get a single history entry for a task in the scoped todolist. Requires task_id and history_id. Returns change content."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID"), mcp.Pattern("^[0-9]+$")),
		mcp.WithString("history_id", mcp.Required(), mcp.Description("History ID"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTodolistGetTool() mcp.Tool {
	return mcp.NewTool("todolist_get",
		mcp.WithDescription("Get the scoped todolist details (read-only). No input required — injected from PROOFHUB_PROJECT_ID/PROOFHUB_TODOLIST_ID. Returns title, privacy, archived, counts."),
	)
}

func newLabelListTool() mcp.Tool {
	return mcp.NewTool("label_list",
		mcp.WithDescription("List all labels (global, not scoped to todolist). No input required. Returns id, name, color."),
	)
}

func newLabelGetTool() mcp.Tool {
	return mcp.NewTool("label_get",
		mcp.WithDescription("Get a single label by ID. Requires label_id (digits). Global lookup, not bound to ENV todolist."),
		mcp.WithString("label_id", mcp.Required(), mcp.Description("Label ID (digits)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newTimesheetListTool() mcp.Tool {
	return mcp.NewTool("timesheet_list",
		mcp.WithDescription("List timesheets in the scoped project (PROOFHUB_PROJECT_ID). No project input required — injected from ENV. Use to lookup timesheets before logging time."),
	)
}

func newTimesheetGetTool() mcp.Tool {
	return mcp.NewTool("timesheet_get",
		mcp.WithDescription("Get a single timesheet in the scoped project. Requires timesheet_id (digits). Project injected from ENV."),
		mcp.WithString("timesheet_id", mcp.Required(), mcp.Description("Timesheet ID (digits)"), mcp.Pattern("^[0-9]+$")),
	)
}

func newPeopleListTool() mcp.Tool {
	return mcp.NewTool("people_list",
		mcp.WithDescription("List all people in ProofHub account (global). Use to lookup assignee IDs for task_create/task_update/subtask. Returns id, first_name, last_name, email. No input required."),
	)
}

// --- handlers scoped to ENV project/todolist ---

func handleTaskList(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
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

func handleTaskGet(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
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

func handleTaskCreate(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		title, err := req.RequireString("title")
		if err != nil {
			return errorResult("missing title", err)
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

func handleTaskUpdate(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
		}
		var payload proofhub.UpdateTaskRequest
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

func handleTaskDelete(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		if err := c.DeleteTask(tctx, projectID, todolistID, taskID); err != nil {
			return errorResult("delete task failed", err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("deleted task %s", taskID)), nil
	}
}

func handleTaskCopy(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
		}
		var payload proofhub.CopyTaskRequest
		if v := getOptionalString(req, "title"); v != "" {
			payload.Title = v
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
		// Enforce single-todolist scope: copy within same project/todolist
		// Still send copy_task id, but disallow new project/list
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

func handleTaskMove(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
		}
		var payload proofhub.MoveTaskRequest
		if hasArg(req, "title") {
			s := getOptionalString(req, "title")
			payload.Title = &s
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

func handleSubtaskList(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
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

func handleSubtaskGet(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		subtaskID, err := req.RequireString("subtask_id")
		if err != nil {
			return errorResult("missing subtask_id", err)
		}
		if !isDigits(taskID) || !isDigits(subtaskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or subtask_id: want digits"))
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

func handleSubtaskCreate(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		title, err := req.RequireString("title")
		if err != nil {
			return errorResult("missing title", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
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

func handleSubtaskUpdate(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		subtaskID, err := req.RequireString("subtask_id")
		if err != nil {
			return errorResult("missing subtask_id", err)
		}
		if !isDigits(taskID) || !isDigits(subtaskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or subtask_id: want digits"))
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

func handleSubtaskDelete(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		subtaskID, err := req.RequireString("subtask_id")
		if err != nil {
			return errorResult("missing subtask_id", err)
		}
		if !isDigits(taskID) || !isDigits(subtaskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or subtask_id: want digits"))
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		if err := c.DeleteSubtask(tctx, projectID, todolistID, taskID, subtaskID); err != nil {
			return errorResult("delete subtask failed", err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("deleted subtask %s", subtaskID)), nil
	}
}

func handleCommentList(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
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

func handleCommentGet(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		commentID, err := req.RequireString("comment_id")
		if err != nil {
			return errorResult("missing comment_id", err)
		}
		if !isDigits(taskID) || !isDigits(commentID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or comment_id: want digits"))
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

func handleCommentCreate(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		desc, err := req.RequireString("description")
		if err != nil {
			return errorResult("missing description", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
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

func handleCommentUpdate(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		commentID, err := req.RequireString("comment_id")
		if err != nil {
			return errorResult("missing comment_id", err)
		}
		desc, err := req.RequireString("description")
		if err != nil {
			return errorResult("missing description", err)
		}
		if !isDigits(taskID) || !isDigits(commentID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or comment_id: want digits"))
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

func handleCommentDelete(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		commentID, err := req.RequireString("comment_id")
		if err != nil {
			return errorResult("missing comment_id", err)
		}
		if !isDigits(taskID) || !isDigits(commentID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or comment_id: want digits"))
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		if err := c.DeleteComment(tctx, projectID, todolistID, taskID, commentID); err != nil {
			return errorResult("delete comment failed", err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("deleted comment %s", commentID)), nil
	}
}

func handleHistoryList(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		if !isDigits(taskID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id %q: want numeric id", taskID))
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

func handleHistoryGet(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult("missing task_id", err)
		}
		historyID, err := req.RequireString("history_id")
		if err != nil {
			return errorResult("missing history_id", err)
		}
		if !isDigits(taskID) || !isDigits(historyID) {
			return errorResult("validation failed", fmt.Errorf("invalid task_id or history_id: want digits"))
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

func handleTodolistGet(client *proofhub.Client, projectID, todolistID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
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

func handleLabelGet(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		labelID, err := req.RequireString("label_id")
		if err != nil {
			return errorResult("missing label_id", err)
		}
		if !isDigits(labelID) {
			return errorResult("validation failed", fmt.Errorf("invalid label_id %q: want numeric id", labelID))
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

func handleTimesheetList(client *proofhub.Client, projectID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
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

func handleTimesheetGet(client *proofhub.Client, projectID string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		timesheetID, err := req.RequireString("timesheet_id")
		if err != nil {
			return errorResult("missing timesheet_id", err)
		}
		if !isDigits(timesheetID) {
			return errorResult("validation failed", fmt.Errorf("invalid timesheet_id %q: want numeric id", timesheetID))
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

func handlePeopleList(client *proofhub.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, err := mustClient(client)
		if err != nil {
			return errorResult("client not configured", err)
		}
		tctx, cancel := withTimeout(ctx)
		defer cancel()
		people, err := c.ListPeople(tctx)
		if err != nil {
			return errorResult("list people failed", err)
		}
		return jsonResult(people)
	}
}
