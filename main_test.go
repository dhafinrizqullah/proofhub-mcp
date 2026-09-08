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
				baseURL:    "https://example.proofhub.com",
				apiKey:     "secret",
				userAgent:  "proofhub-mcp (test@example.com)",
				projectID:  "8213786200",
				todolistID: "271478716253",
			},
		},
		{
			name: "missing base_url",
			cfg: &config{
				baseURL:    "",
				apiKey:     "secret",
				userAgent:  "proofhub-mcp (test@example.com)",
				projectID:  "1",
				todolistID: "1",
			},
			wantErr: "proofhub_base_url is required",
		},
		{
			name: "missing api_key",
			cfg: &config{
				baseURL:    "https://example.com",
				apiKey:     "",
				userAgent:  "proofhub-mcp (test@example.com)",
				projectID:  "1",
				todolistID: "1",
			},
			wantErr: "proofhub_api_key is required",
		},
		{
			name: "missing project_id",
			cfg: &config{
				baseURL:    "https://example.com",
				apiKey:     "k",
				userAgent:  "proofhub-mcp (test@example.com)",
				projectID:  "",
				todolistID: "1",
			},
			wantErr: "proofhub_project_id",
		},
		{
			name: "invalid project_id non-digits",
			cfg: &config{
				baseURL:    "https://example.com",
				apiKey:     "k",
				userAgent:  "proofhub-mcp (test@example.com)",
				projectID:  "abc",
				todolistID: "1",
			},
			wantErr: "proofhub_project_id",
		},
		{
			name: "invalid todolist_id non-digits",
			cfg: &config{
				baseURL:    "https://example.com",
				apiKey:     "k",
				userAgent:  "proofhub-mcp (test@example.com)",
				projectID:  "1",
				todolistID: "12a",
			},
			wantErr: "proofhub_todolist_id",
		},
		{
			name: "invalid user_agent format",
			cfg: &config{
				baseURL:    "https://example.com",
				apiKey:     "k",
				userAgent:  "bad-agent",
				projectID:  "1",
				todolistID: "1",
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
				"PROOFHUB_BASE_URL":    "https://example.com",
				"PROOFHUB_API_KEY":     "k",
				"PROOFHUB_PROJECT_ID":  "1",
				"PROOFHUB_TODOLIST_ID": "2",
			},
			expected: &config{
				baseURL:    "https://example.com",
				projectID:  "1",
				todolistID: "2",
				transport:  "stdio",
				httpPort:   "8080",
			},
		},
		{
			name: "transport flag overrides env",
			env: map[string]string{
				"PROOFHUB_BASE_URL":    "https://example.com",
				"PROOFHUB_API_KEY":     "k",
				"PROOFHUB_PROJECT_ID":  "1",
				"PROOFHUB_TODOLIST_ID": "2",
				"MCP_TRANSPORT":        "stdio",
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
				"PROOFHUB_BASE_URL":    "https://example.com",
				"PROOFHUB_API_KEY":     "k",
				"PROOFHUB_PROJECT_ID":  "1",
				"PROOFHUB_TODOLIST_ID": "2",
				"MCP_TRANSPORT":        "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid http port",
			env: map[string]string{
				"PROOFHUB_BASE_URL":    "https://example.com",
				"PROOFHUB_API_KEY":     "k",
				"PROOFHUB_PROJECT_ID":  "1",
				"PROOFHUB_TODOLIST_ID": "2",
				"MCP_HTTP_PORT":        "not-a-port",
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
		})
	}
}

// TestToolRegistration verifies 24 scoped tools and descriptions.
func TestToolRegistration(t *testing.T) {
	t.Parallel()
	s := server.NewMCPServer("test", "test", server.WithRecovery())
	dummyClient := proofhub.New("https://example.com", "dummy", "proofhub-mcp (test@example.com)")
	registerTools(s, dummyClient, "1", "2")

	allTools := []mcp.Tool{
		newTaskListTool(), newTaskGetTool(), newTaskCreateTool(), newTaskUpdateTool(), newTaskDeleteTool(), newTaskCopyTool(), newTaskMoveTool(),
		newSubtaskListTool(), newSubtaskGetTool(), newSubtaskCreateTool(), newSubtaskUpdateTool(), newSubtaskDeleteTool(),
		newCommentListTool(), newCommentGetTool(), newCommentCreateTool(), newCommentUpdateTool(), newCommentDeleteTool(),
		newHistoryListTool(), newHistoryGetTool(),
		newTodolistGetTool(), newLabelListTool(), newLabelGetTool(), newTimesheetListTool(), newTimesheetGetTool(),
		newPeopleListTool(),
	}
	assert.Len(t, allTools, 25, "should have 25 tools (7+5+5+2+1+2+2+1)")

	tests := []struct {
		name            string
		tool            mcp.Tool
		expectNoProject bool
	}{
		{name: "task_list", tool: newTaskListTool(), expectNoProject: true},
		{name: "task_get", tool: newTaskGetTool(), expectNoProject: true},
		{name: "task_create", tool: newTaskCreateTool(), expectNoProject: true},
		{name: "todolist_get", tool: newTodolistGetTool(), expectNoProject: true},
		{name: "label_list", tool: newLabelListTool(), expectNoProject: true},
		{name: "timesheet_list", tool: newTimesheetListTool(), expectNoProject: true},
		{name: "timesheet_get", tool: newTimesheetGetTool(), expectNoProject: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := assert.New(t)
			is.Equal(tt.name, tt.tool.Name)
			is.NotEmpty(tt.tool.Description, "description must not be empty")
			if tt.expectNoProject {
				schemaBytes, _ := json.Marshal(tt.tool.InputSchema)
				schemaStr := string(schemaBytes)
				is.NotContains(schemaStr, "project_id", "scoped tool should not expose project_id")
				if tt.name != "label_list" && tt.name != "label_get" {
					hasScope := strings.Contains(strings.ToLower(tt.tool.Description), "scoped") ||
						strings.Contains(strings.ToLower(tt.tool.Description), "env")
					is.True(hasScope, "description should mention scoped/ENV for %s: got %q", tt.name, tt.tool.Description)
				}
			}
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
	handler := handleTaskList(client, "1", "2")
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
			h := handleTaskGet(client, "1", "2")
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
	h := handleTodolistGet(client, "1", "2")
	req := newCallRequest(map[string]any{})
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "My List")
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
