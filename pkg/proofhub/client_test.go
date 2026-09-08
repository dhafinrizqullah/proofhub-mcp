package proofhub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{name: "plain domain", baseURL: "https://example.proofhub.com", expected: "https://example.proofhub.com/api/v3"},
		{name: "with trailing slash", baseURL: "https://example.proofhub.com/", expected: "https://example.proofhub.com/api/v3"},
		{name: "with api v3", baseURL: "https://example.proofhub.com/api/v3", expected: "https://example.proofhub.com/api/v3"},
		{name: "with api v3 slash", baseURL: "https://example.proofhub.com/api/v3/", expected: "https://example.proofhub.com/api/v3"},
		{name: "with api v3 and path", baseURL: "https://example.proofhub.com/api/v3/some", expected: "https://example.proofhub.com/api/v3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := New(tt.baseURL, "key", "proofhub-mcp (test@example.com)")
			assert.Equal(t, tt.expected, c.BaseURL)
			assert.Equal(t, 3, c.MaxRetries)
			assert.Equal(t, time.Second, c.BaseRetryDelay)
		})
	}
	t.Run("default user agent", func(t *testing.T) {
		t.Parallel()
		c := New("https://example.com", "k", "")
		assert.Equal(t, "proofhub-mcp (dev@example.com)", c.UserAgent)
	})
	t.Run("with options", func(t *testing.T) {
		t.Parallel()
		c := New("https://example.com", "k", "App (a@b.com)", WithMaxRetries(5), WithBaseRetryDelay(2*time.Second))
		assert.Equal(t, 5, c.MaxRetries)
		assert.Equal(t, 2*time.Second, c.BaseRetryDelay)
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		client  *Client
		wantErr string
	}{
		{
			name:    "valid",
			client:  New("https://example.com", "key", "MyApp (user@example.com)"),
			wantErr: "",
		},
		{
			name:    "missing base_url",
			client:  &Client{BaseURL: "", APIKey: "k", UserAgent: "App (a@b.com)"},
			wantErr: "base URL is required",
		},
		{
			name:    "missing api_key",
			client:  &Client{BaseURL: "https://example.com", APIKey: "", UserAgent: "App (a@b.com)"},
			wantErr: "API key is required",
		},
		{
			name:    "invalid user_agent",
			client:  &Client{BaseURL: "https://example.com", APIKey: "k", UserAgent: "bad"},
			wantErr: "user-agent must look like",
		},
		{
			name:    "user_agent missing email",
			client:  &Client{BaseURL: "https://example.com", APIKey: "k", UserAgent: "App (notanemail)"},
			wantErr: "user-agent must look like",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.client.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
			// Ensure no secret leak in this validation (api key not echoed)
			assert.NotContains(t, err.Error(), "secret-api-key")
		})
	}
}

func TestRetryableStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code     int
		expected bool
	}{
		{code: 429, expected: true},
		{code: 500, expected: true},
		{code: 502, expected: true},
		{code: 503, expected: true},
		{code: 504, expected: true},
		{code: 400, expected: false},
		{code: 401, expected: false},
		{code: 404, expected: false},
		{code: 200, expected: false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.code), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, retryableStatus(tt.code))
		})
	}
}

func TestBackoff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		base     time.Duration
		attempt  int
		expected time.Duration
	}{
		{name: "attempt 1", base: time.Second, attempt: 1, expected: time.Second},
		{name: "attempt 2", base: time.Second, attempt: 2, expected: 2 * time.Second},
		{name: "attempt 3", base: time.Second, attempt: 3, expected: 4 * time.Second},
		{name: "capped", base: time.Second, attempt: 10, expected: maxBackoff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := backoff(tt.base, tt.attempt)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{name: "empty", input: "", expected: 0},
		{name: "seconds", input: "30", expected: 30 * time.Second},
		{name: "seconds capped", input: "9999", expected: maxRetryAfter},
		{name: "negative", input: "-5", expected: 0},
		{name: "invalid", input: "not-a-number", expected: 0},
		{name: "http date future", input: time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat), expected: 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseRetryAfter(tt.input)
			// For http date, allow small delta
			if tt.name == "http date future" {
				assert.WithinDuration(t, time.Now().Add(tt.expected), time.Now().Add(got), time.Second)
				return
			}
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel...", truncate("hello world", 3))
}

func TestAPIError(t *testing.T) {
	t.Parallel()
	err := &APIError{StatusCode: 429, Body: "rate limited", RetryAfter: 5 * time.Second}
	assert.Contains(t, err.Error(), "429")
	assert.Contains(t, err.Error(), "5s")

	err2 := &APIError{StatusCode: 500, Body: "server error"}
	assert.Contains(t, err2.Error(), "500")
	var target *APIError
	assert.ErrorAs(t, err, &target)
}

func TestDo_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "dummy-key", r.Header.Get("X-API-KEY"))
		assert.Contains(t, r.Header.Get("User-Agent"), "proofhub-mcp")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer srv.Close()

	c := New(srv.URL, "dummy-key", "proofhub-mcp (test@example.com)")
	c.HTTP = srv.Client()
	body, code, err := c.do(context.Background(), http.MethodGet, "/test", nil)
	require.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Contains(t, string(body), "ok")
}

func TestDo_RetryOn429ThenSuccess(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "App (a@b.com)")
	c.HTTP = srv.Client()
	c.MaxRetries = 1
	c.BaseRetryDelay = time.Millisecond

	body, code, err := c.do(context.Background(), http.MethodGet, "/test", nil)
	require.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Equal(t, 2, calls)
	assert.Contains(t, string(body), `"id":1`)
}

func TestDo_NoRetryOn400(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "App (a@b.com)")
	c.HTTP = srv.Client()
	c.MaxRetries = 3
	_, code, err := c.do(context.Background(), http.MethodGet, "/test", nil)
	require.Error(t, err)
	var apiErr *APIError
	assert.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, code)
	assert.Equal(t, 1, calls, "should not retry on 400")
}

func TestDo_ContextCancel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "App (a@b.com)")
	c.HTTP = srv.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, _, err := c.do(ctx, http.MethodGet, "/test", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline"), "should wrap context error: %v", err)
}

func TestCreateTask_Validation(t *testing.T) {
	t.Parallel()
	c := New("https://example.com", "k", "App (a@b.com)")
	_, err := c.CreateTask(context.Background(), "1", "2", CreateTaskRequest{Title: ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestCreateComment_Validation(t *testing.T) {
	t.Parallel()
	c := New("https://example.com", "k", "App (a@b.com)")
	_, err := c.CreateComment(context.Background(), "1", "2", "3", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "comment description is required")
}

func TestListTasks_Decode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/projects/1/todolists/2/tasks", r.URL.Path)
		_ = json.NewEncoder(w).Encode([]Task{{ID: 123, Title: "hello"}})
	}))
	defer srv.Close()
	c := New(srv.URL, "k", "App (a@b.com)")
	c.HTTP = srv.Client()
	tasks, err := c.ListTasks(context.Background(), "1", "2")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, int64(123), tasks[0].ID)
}

func TestGetLabel_Decode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Label{ID: 999, Name: "Urgent", Color: "#FF0000"})
	}))
	defer srv.Close()
	c := New(srv.URL, "k", "App (a@b.com)")
	c.HTTP = srv.Client()
	label, err := c.GetLabel(context.Background(), "999")
	require.NoError(t, err)
	assert.Equal(t, int64(999), label.ID)
	assert.Equal(t, "Urgent", label.Name)
}
