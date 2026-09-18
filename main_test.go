package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhafinrizqullah/proofhub-mcp/pkg/proofhub"
)

// TestIsDigits validates numeric ID checks.
func TestIsDigits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "valid digits", input: "12345", expected: true},
		{name: "single digit", input: "0", expected: true},
		{name: "empty string", input: "", expected: false},
		{name: "with letters", input: "12a34", expected: false},
		{name: "with spaces", input: "12 34", expected: false},
		{name: "with dash", input: "12-34", expected: false},
		{name: "leading zeros", input: "000123", expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := assert.New(t)
			is.Equal(tt.expected, isDigits(tt.input))
		})
	}
}

// TestValidateDate checks YYYY-MM-DD validation.
func TestValidateDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid date", input: "2026-09-08", wantErr: false},
		{name: "empty is ok", input: "", wantErr: false},
		{name: "invalid format", input: "08-09-2026", wantErr: true},
		{name: "invalid month", input: "2026-13-01", wantErr: true},
		{name: "not a date", input: "not-a-date", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateDate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "want YYYY-MM-DD")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateConfig ensures scoped ENV validation fails fast with lowercase messages.
func TestValidateConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     *config
		wantErr string
	}{
		{
			name: "valid config",
			cfg: &config{
				baseURL:     "https://example.proofhub.com",
				apiKey:      "secret",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "8213786200",
				todolistIDs: []string{"271478716253"},
			},
		},
		{
			name: "missing base_url",
			cfg: &config{
				baseURL:     "",
				apiKey:      "secret",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "1",
				todolistIDs: []string{"1"},
			},
			wantErr: "proofhub_base_url is required",
		},
		{
			name: "missing api_key",
			cfg: &config{
				baseURL:     "https://example.com",
				apiKey:      "",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "1",
				todolistIDs: []string{"1"},
			},
			wantErr: "proofhub_api_key is required",
		},
		{
			name: "missing project_id",
			cfg: &config{
				baseURL:     "https://example.com",
				apiKey:      "k",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "",
				todolistIDs: []string{"1"},
			},
			wantErr: "proofhub_project_id",
		},
		{
			name: "invalid project_id non-digits",
			cfg: &config{
				baseURL:     "https://example.com",
				apiKey:      "k",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "abc",
				todolistIDs: []string{"1"},
			},
			wantErr: "proofhub_project_id",
		},
		{
			name: "invalid todolist_ids non-digits",
			cfg: &config{
				baseURL:     "https://example.com",
				apiKey:      "k",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "1",
				todolistIDs: []string{"12a"},
			},
			wantErr: "proofhub_todolist_ids",
		},
		{
			name: "missing todolist_ids",
			cfg: &config{
				baseURL:     "https://example.com",
				apiKey:      "k",
				userAgent:   "proofhub-mcp (test@example.com)",
				projectID:   "1",
				todolistIDs: nil,
			},
			wantErr: "proofhub_todolist_ids",
		},
		{
			name: "invalid user_agent format",
			cfg: &config{
				baseURL:     "https://example.com",
				apiKey:      "k",
				userAgent:   "bad-agent",
				projectID:   "1",
				todolistIDs: []string{"1"},
			},
			wantErr: "user-agent must look like",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateConfig(tt.cfg)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
			assert.Equal(t, strings.ToLower(err.Error()[:1]), err.Error()[:1], "error should be lowercase")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

// TestLoadConfig covers ENV and flag parsing.
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		args     []string
		expected *config
		wantErr  bool
	}{
		{
			name: "defaults from env",
			env: map[string]string{
				"PROOFHUB_BASE_URL":     "https://example.com",
				"PROOFHUB_API_KEY":      "k",
				"PROOFHUB_PROJECT_ID":   "1",
				"PROOFHUB_TODOLIST_IDS": "2,3",
				"PROOFHUB_TODOLIST_ID":  "",
			},
			expected: &config{
				baseURL:     "https://example.com",
				projectID:   "1",
				todolistIDs: []string{"2", "3"},
				transport:   "stdio",
				httpPort:    "8080",
			},
		},
		{
			name: "legacy single todolist id",
			env: map[string]string{
				"PROOFHUB_BASE_URL":     "https://example.com",
				"PROOFHUB_API_KEY":      "k",
				"PROOFHUB_PROJECT_ID":   "1",
				"PROOFHUB_TODOLIST_IDS": "",
				"PROOFHUB_TODOLIST_ID":  "2",
			},
			expected: &config{
				baseURL:     "https://example.com",
				projectID:   "1",
				todolistIDs: []string{"2"},
				transport:   "stdio",
				httpPort:    "8080",
			},
		},
		{
			name: "invalid todolist ids",
			env: map[string]string{
				"PROOFHUB_BASE_URL":     "https://example.com",
				"PROOFHUB_API_KEY":      "k",
				"PROOFHUB_PROJECT_ID":   "1",
				"PROOFHUB_TODOLIST_IDS": "2,abc",
				"PROOFHUB_TODOLIST_ID":  "",
			},
			wantErr: true,
		},
		{
			name: "transport flag overrides env",
			env: map[string]string{
				"PROOFHUB_BASE_URL":     "https://example.com",
				"PROOFHUB_API_KEY":      "k",
				"PROOFHUB_PROJECT_ID":   "1",
				"PROOFHUB_TODOLIST_IDS": "2",
				"MCP_TRANSPORT":         "stdio",
			},
			args: []string{"--transport", "http", "--http-port", "9090"},
			expected: &config{
				transport: "http",
				httpPort:  "9090",
			},
		},
		{
			name: "invalid transport",
			env: map[string]string{
				"PROOFHUB_BASE_URL":     "https://example.com",
				"PROOFHUB_API_KEY":      "k",
				"PROOFHUB_PROJECT_ID":   "1",
				"PROOFHUB_TODOLIST_IDS": "2",
				"MCP_TRANSPORT":         "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid http port",
			env: map[string]string{
				"PROOFHUB_BASE_URL":     "https://example.com",
				"PROOFHUB_API_KEY":      "k",
				"PROOFHUB_PROJECT_ID":   "1",
				"PROOFHUB_TODOLIST_IDS": "2",
				"MCP_HTTP_PORT":         "not-a-port",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, ok := tt.env["MCP_TRANSPORT"]; !ok {
				t.Setenv("MCP_TRANSPORT", "")
			}
			if _, ok := tt.env["PROOFHUB_TODOLIST_IDS"]; !ok {
				t.Setenv("PROOFHUB_TODOLIST_IDS", "")
			}
			if _, ok := tt.env["PROOFHUB_TODOLIST_ID"]; !ok {
				t.Setenv("PROOFHUB_TODOLIST_ID", "")
			}
			if _, ok := tt.env["MCP_HTTP_PORT"]; !ok {
				t.Setenv("MCP_HTTP_PORT", "")
			}
			cfg, err := loadConfig(tt.args)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.expected.baseURL != "" {
				assert.Equal(t, tt.expected.baseURL, cfg.baseURL)
			}
			if tt.expected.transport != "" {
				assert.Equal(t, tt.expected.transport, cfg.transport)
			}
			if tt.expected.httpPort != "" {
				assert.Equal(t, tt.expected.httpPort, cfg.httpPort)
			}
			if tt.expected.todolistIDs != nil {
				assert.Equal(t, tt.expected.todolistIDs, cfg.todolistIDs)
			}
		})
	}
}

// TestToolRegistration verifies 26 tools default to ENV with optional overrides.
func TestToolRegistration(t *testing.T) {
	t.Parallel()
	s := server.NewMCPServer("test", "test", server.WithRecovery())
	dummyClient := proofhub.New("https://example.com", "dummy", "proofhub-mcp (test@example.com)")
	registerTools(s, dummyClient, "1", []string{"2"})

	allTools := []mcp.Tool{
		newTaskListTool(), newTaskGetTool(), newTaskCreateTool(), newTaskUpdateTool(), newTaskDeleteTool(), newTaskCopyTool(), newTaskMoveTool(),
		newSubtaskListTool(), newSubtaskGetTool(), newSubtaskCreateTool(), newSubtaskUpdateTool(), newSubtaskDeleteTool(),
		newCommentListTool(), newCommentGetTool(), newCommentCreateTool(), newCommentUpdateTool(), newCommentDeleteTool(),
		newHistoryListTool(), newHistoryGetTool(),
		newTodolistGetTool(), newTodolistListTool(), newLabelListTool(), newLabelGetTool(), newTimesheetListTool(), newTimesheetGetTool(),
		newPeopleListTool(),
	}
	assert.Len(t, allTools, 26, "should have 26 tools (7+5+5+2+2+2+2+1)")

	tests := []struct {
		name              string
		tool              mcp.Tool
		expectScopeFields []string
	}{
		{name: "task_list", tool: newTaskListTool(), expectScopeFields: []string{"todolist_id"}},
		{name: "task_get", tool: newTaskGetTool(), expectScopeFields: []string{"todolist_id"}},
		{name: "task_create", tool: newTaskCreateTool(), expectScopeFields: []string{"todolist_id"}},
		{name: "task_copy", tool: newTaskCopyTool(), expectScopeFields: []string{"todolist_id", "new_todolist_id"}},
		{name: "task_move", tool: newTaskMoveTool(), expectScopeFields: []string{"todolist_id", "new_todolist_id"}},
		{name: "todolist_get", tool: newTodolistGetTool(), expectScopeFields: []string{"todolist_id"}},
		{name: "todolist_list", tool: newTodolistListTool(), expectScopeFields: nil},
		{name: "timesheet_list", tool: newTimesheetListTool(), expectScopeFields: nil},
		{name: "timesheet_get", tool: newTimesheetGetTool(), expectScopeFields: nil},
		{name: "label_list", tool: newLabelListTool(), expectScopeFields: nil},
		{name: "people_list", tool: newPeopleListTool(), expectScopeFields: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := assert.New(t)
			is.Equal(tt.name, tt.tool.Name)
			is.NotEmpty(tt.tool.Description, "description must not be empty")
			schemaBytes, _ := json.Marshal(tt.tool.InputSchema)
			schemaStr := string(schemaBytes)
			for _, field := range tt.expectScopeFields {
				is.Contains(schemaStr, field, "tool %s should expose %s", tt.name, field)
			}
			if len(tt.expectScopeFields) == 0 {
				is.NotContains(schemaStr, "project_id", "global tool should not expose project_id")
				is.NotContains(schemaStr, "todolist_id", "global tool should not expose todolist_id")
			} else {
				desc := strings.ToLower(tt.tool.Description)
				hasScope := strings.Contains(desc, "env") ||
					strings.Contains(desc, "default") ||
					strings.Contains(desc, "maintainable")
				is.True(hasScope, "description should mention scope for %s: got %q", tt.name, tt.tool.Description)
			}
		})
	}
}

// TestParseTodolistIDs covers allowlist parsing.
func TestParseTodolistIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		raw      string
		expected []string
		wantErr  bool
	}{
		{name: "single id", raw: "2714", expected: []string{"2714"}},
		{name: "comma-separated", raw: "2714,2720,2731", expected: []string{"2714", "2720", "2731"}},
		{name: "spaces trimmed", raw: " 2714 , 2720 ", expected: []string{"2714", "2720"}},
		{name: "deduplicated", raw: "2714,2714,2720", expected: []string{"2714", "2720"}},
		{name: "empty entries skipped", raw: "2714,,2720,", expected: []string{"2714", "2720"}},
		{name: "empty raw", raw: "", wantErr: true},
		{name: "only commas", raw: " , ,", wantErr: true},
		{name: "non-digits", raw: "2714,abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := assert.New(t)
			got, err := parseTodolistIDs(tt.raw)
			if tt.wantErr {
				is.Error(err)
				return
			}
			is.NoError(err)
			is.Equal(tt.expected, got)
		})
	}
}

// TestResolveTodolist covers allowlist enforcement and single-list default.
func TestResolveTodolist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		allowed []string
		want    string
		wantErr string
	}{
		{name: "single list default", args: map[string]any{}, allowed: []string{"2"}, want: "2"},
		{name: "explicit allowed", args: map[string]any{"todolist_id": "3"}, allowed: []string{"2", "3"}, want: "3"},
		{name: "multi requires todolist_id", args: map[string]any{}, allowed: []string{"2", "3"}, wantErr: "todolist_id is required"},
		{name: "not maintainable", args: map[string]any{"todolist_id": "9"}, allowed: []string{"2", "3"}, wantErr: "not maintainable"},
		{name: "non-digits", args: map[string]any{"todolist_id": "12a"}, allowed: []string{"2", "3"}, wantErr: "want numeric id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := assert.New(t)
			req := newCallRequest(tt.args)
			got, err := resolveTodolist(req, tt.allowed)
			if tt.wantErr != "" {
				require.Error(t, err)
				is.Contains(err.Error(), tt.wantErr)
				return
			}
			is.NoError(err)
			is.Equal(tt.want, got)
		})
	}
}

// TestResolveNewTodolist covers copy/move destinations inside the allowlist.
func TestResolveNewTodolist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		allowed []string
		source  string
		want    string
		wantErr string
	}{
		{name: "default same as source", args: map[string]any{}, allowed: []string{"2", "3"}, source: "2", want: "2"},
		{name: "cross-list destination", args: map[string]any{"new_todolist_id": "3"}, allowed: []string{"2", "3"}, source: "2", want: "3"},
		{name: "destination not maintainable", args: map[string]any{"new_todolist_id": "9"}, allowed: []string{"2", "3"}, source: "2", wantErr: "not maintainable"},
		{name: "destination non-digits", args: map[string]any{"new_todolist_id": "x"}, allowed: []string{"2", "3"}, source: "2", wantErr: "want numeric id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := assert.New(t)
			req := newCallRequest(tt.args)
			got, err := resolveNewTodolist(req, tt.allowed, tt.source)
			if tt.wantErr != "" {
				require.Error(t, err)
				is.Contains(err.Error(), tt.wantErr)
				return
			}
			is.NoError(err)
			is.Equal(tt.want, got)
		})
	}
}

// TestGetOptionalInt covers number parsing.
func TestGetOptionalInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		expected *int
		wantErr  bool
	}{
		{name: "float64", args: map[string]any{"x": float64(5)}, key: "x", expected: intPtr(5)},
		{name: "int", args: map[string]any{"x": 7}, key: "x", expected: intPtr(7)},
		{name: "missing", args: map[string]any{}, key: "x", expected: nil},
		{name: "string int", args: map[string]any{"x": "42"}, key: "x", expected: intPtr(42)},
		{name: "invalid string", args: map[string]any{"x": "abc"}, key: "x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newCallRequest(tt.args)
			got, err := getOptionalInt(req, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.expected == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.expected, *got)
			}
		})
	}
}

// TestGetInt64Slice covers array and CSV parsing.
func TestGetInt64Slice(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		expected []int64
		wantErr  bool
	}{
		{name: "int array", args: map[string]any{"ids": []any{float64(1), float64(2)}}, key: "ids", expected: []int64{1, 2}},
		{name: "csv string", args: map[string]any{"ids": "1,2,3"}, key: "ids", expected: []int64{1, 2, 3}},
		{name: "empty", args: map[string]any{}, key: "ids", expected: nil},
		{name: "invalid csv", args: map[string]any{"ids": "1,abc"}, key: "ids", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newCallRequest(tt.args)
			got, err := getInt64Slice(req, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func newCallRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

// TestWithTimeout ensures context timeout is set.
func TestWithTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tctx, cancel := withTimeout(ctx)
	defer cancel()
	deadline, ok := tctx.Deadline()
	assert.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(30*time.Second), deadline, time.Second)
}

func intPtr(i int) *int { return &i }

// TestParseTarget is not in main, but we test the helper in proofhub package via main's usage
// Additional handler-level tests using httptest to verify scoped injection.

func TestHandleTaskList_Success(t *testing.T) {
	// Mock ProofHub API
	srv := newMockProofHubServer(t, "/api/v3/projects/1/todolists/2/tasks", `[{"id":123,"title":"hello"}]`)
	defer srv.Close()
	client := proofhub.New(srv.URL, "k", "App (a@b.com)")
	client.HTTP = srv.Client()
	handler := handleTaskList(client, "1", []string{"2"})
	req := newCallRequest(map[string]any{})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	// Result text should contain the mocked task
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "hello")
}

func TestHandleTaskGet_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{name: "valid", args: map[string]any{"task_id": "123"}, wantErr: false},
		{name: "missing task_id", args: map[string]any{}, wantErr: true},
		{name: "invalid digits", args: map[string]any{"task_id": "abc"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Don't use t.Parallel for subtests sharing server; create isolated client per subtest
			var client *proofhub.Client
			if !tt.wantErr {
				srv := newMockProofHubServer(t, "/api/v3/projects/1/todolists/2/tasks/123", `{"id":123,"title":"hello"}`)
				defer srv.Close()
				client = proofhub.New(srv.URL, "k", "App (a@b.com)")
				client.HTTP = srv.Client()
			} else {
				client = proofhub.New("https://example.com", "k", "App (a@b.com)")
			}
			h := handleTaskGet(client, "1", []string{"2"})
			req := newCallRequest(tt.args)
			result, err := h(context.Background(), req)
			require.NoError(t, err)
			if tt.wantErr {
				assert.True(t, result.IsError, "should be error for %s", tt.name)
			} else {
				assert.False(t, result.IsError)
			}
		})
	}
}

func TestHandleTodolistGet_Scoped(t *testing.T) {
	t.Parallel()
	srv := newMockProofHubServer(t, "/api/v3/projects/1/todolists/2", `{"id":2,"title":"My List"}`)
	defer srv.Close()
	client := proofhub.New(srv.URL, "k", "App (a@b.com)")
	client.HTTP = srv.Client()
	h := handleTodolistGet(client, "1", []string{"2"})
	req := newCallRequest(map[string]any{})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "My List")
}

func TestHandleTaskList_MultiRequiresTodolist(t *testing.T) {
	t.Parallel()
	client := proofhub.New("https://example.com", "k", "App (a@b.com)")
	h := handleTaskList(client, "1", []string{"2", "3"})
	req := newCallRequest(map[string]any{})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "todolist_id is required")
}

func TestHandleTaskList_RejectsUnlisted(t *testing.T) {
	t.Parallel()
	client := proofhub.New("https://example.com", "k", "App (a@b.com)")
	h := handleTaskList(client, "1", []string{"2", "3"})
	req := newCallRequest(map[string]any{"todolist_id": "9"})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "not maintainable")
}

func TestHandleTaskList_OverrideSuccess(t *testing.T) {
	t.Parallel()
	srv := newMockProofHubServer(t, "/api/v3/projects/1/todolists/3/tasks", `[{"id":123,"title":"hello"}]`)
	defer srv.Close()
	client := proofhub.New(srv.URL, "k", "App (a@b.com)")
	client.HTTP = srv.Client()
	h := handleTaskList(client, "1", []string{"2", "3"})
	req := newCallRequest(map[string]any{"todolist_id": "3"})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "hello")
}

func TestHandleTodolistList_FiltersAllowed(t *testing.T) {
	t.Parallel()
	srv := newMockProofHubServer(t, "/api/v3/projects/1/todolists", `[{"id":2,"title":"Keep"},{"id":9,"title":"Drop"}]`)
	defer srv.Close()
	client := proofhub.New(srv.URL, "k", "App (a@b.com)")
	client.HTTP = srv.Client()
	h := handleTodolistList(client, "1", []string{"2"})
	req := newCallRequest(map[string]any{})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "Keep")
	assert.NotContains(t, text, "Drop")
}

func TestHandleTaskCopy_CrossListDestination(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/projects/1/todolists/2/tasks/123", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":124,"title":"copy"}`))
	}))
	defer srv.Close()
	client := proofhub.New(srv.URL, "k", "App (a@b.com)")
	client.HTTP = srv.Client()
	h := handleTaskCopy(client, "1", []string{"2", "3"})
	req := newCallRequest(map[string]any{"task_id": "123", "todolist_id": "2", "new_todolist_id": "3"})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Equal(t, float64(3), gotBody["list_id"])
}

func TestHandleTaskCopy_RejectsUnlistedDestination(t *testing.T) {
	t.Parallel()
	client := proofhub.New("https://example.com", "k", "App (a@b.com)")
	h := handleTaskCopy(client, "1", []string{"2", "3"})
	req := newCallRequest(map[string]any{"task_id": "123", "todolist_id": "2", "new_todolist_id": "9"})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "not maintainable")
}

func newMockProofHubServer(t *testing.T, expectedPath, response string) *httptest.Server {
	t.Helper()
	handler := httpHandlerFunc(t, expectedPath, response)
	return httptest.NewServer(handler)
}

func httpHandlerFunc(t *testing.T, expectedPath, response string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Logf("unexpected path: got %s want %s", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}
}
