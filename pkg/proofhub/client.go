package proofhub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client is a minimal ProofHub API v3 client.
// Docs: https://github.com/ProofHub/api_v3
//
// A Client is safe for concurrent use; it holds no per-request state.
type Client struct {
	BaseURL   string
	APIKey    string
	UserAgent string
	HTTP      *http.Client
	// MaxRetries bounds retries on 429/500/502/503/504. 0 disables retries.
	MaxRetries int
	// BaseRetryDelay is the first backoff wait; it doubles per attempt (capped).
	BaseRetryDelay time.Duration
}

// Option configures a Client. Options never fail; use Validate for checks.
type Option func(*Client)

// WithMaxRetries sets how many times a 429/5xx response is retried (default 3).
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.MaxRetries = n
		}
	}
}

// WithBaseRetryDelay sets the first backoff wait before a retry (default 1s).
func WithBaseRetryDelay(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.BaseRetryDelay = d
		}
	}
}

// New normalizes base URL (accepts "https://xxx.proofhub.com" or ".../api/v3").
func New(baseURL, apiKey, userAgent string, opts ...Option) *Client {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimSuffix(baseURL, "/")
	// Allow passing full base with /api/v3 or /api/v3/
	if strings.HasSuffix(baseURL, "/api/v3") {
		// ok
	} else if strings.Contains(baseURL, "/api/v3/") {
		baseURL = strings.Split(baseURL, "/api/v3")[0] + "/api/v3"
	} else {
		baseURL = baseURL + "/api/v3"
	}
	if userAgent == "" {
		userAgent = "proofhub-mcp (dev@example.com)"
	}
	c := &Client{
		BaseURL:        baseURL,
		APIKey:         apiKey,
		UserAgent:      userAgent,
		HTTP:           &http.Client{Timeout: 30 * time.Second},
		MaxRetries:     3,
		BaseRetryDelay: time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// userAgentPattern enforces ProofHub's "AppName (name@example.com)" format.
// ProofHub answers 400 when User-Agent is missing/malformed, so fail fast.
var userAgentPattern = regexp.MustCompile(`^.+\s\(.+@.+\)$`)

// Validate fails fast on config the server would reject (empty key, bad User-Agent).
func (c *Client) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("base URL is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("API key is required")
	}
	if !userAgentPattern.MatchString(c.UserAgent) {
		return fmt.Errorf("user-agent must look like %q, got %q (proofhub returns 400 without it)",
			"AppName (name@example.com)", c.UserAgent)
	}
	return nil
}

// APIError carries an HTTP failure (status + truncated body). Inspect with errors.As.
type APIError struct {
	StatusCode int
	Body       string
	// RetryAfter is set from the Retry-After header on 429, if present.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.StatusCode == http.StatusTooManyRequests && e.RetryAfter > 0 {
		return fmt.Sprintf("proofhub API 429 rate limited, retry after %s: %s", e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("proofhub API %d: %s", e.StatusCode, e.Body)
}

const (
	// maxBackoff caps exponential backoff between retries.
	maxBackoff = 15 * time.Second
	// maxRetryAfter caps a single wait from the Retry-After header.
	maxRetryAfter = 60 * time.Second
)

// retryableStatus reports which statuses deserve a retry per ProofHub docs:
// 429 (rate limited) and 500/502/503/504 (server-side, "re-trying should solve it").
// Anything else (400/401/403/404/415/...) is returned immediately.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// backoff returns base * 2^(attempt-1), capped at maxBackoff. attempt starts at 1.
func backoff(base time.Duration, attempt int) time.Duration {
	d := base << (attempt - 1)
	if d <= 0 || d > maxBackoff {
		return maxBackoff
	}
	return d
}

// parseRetryAfter reads seconds ("120") or an HTTP date ("Wed, 21 Oct 2015 07:28:00 GMT").
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return min(time.Duration(secs)*time.Second, maxRetryAfter)
	}
	if t, err := http.ParseTime(v); err == nil {
		return min(max(time.Until(t), 0), maxRetryAfter)
	}
	return 0
}

// sleepCtx waits d or aborts early when ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("aborted while waiting %s before retry: %w", d, ctx.Err())
	case <-t.C:
		return nil
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var payload []byte
	if body != nil {
		// Always valid JSON, so the server never answers 415 to our requests.
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		payload = b
	}
	url := c.BaseURL + path
	maxAttempts := c.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	base := c.BaseRetryDelay
	if base <= 0 {
		base = time.Second
	}
	httpc := c.HTTP
	if httpc == nil {
		httpc = http.DefaultClient // tolerate a zero-value Client
	}

	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("X-API-KEY", c.APIKey)
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := httpc.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, fmt.Errorf("request %s %s: %w", method, url, ctx.Err())
			}
			if attempt > c.MaxRetries {
				return nil, 0, fmt.Errorf("request %s %s failed after %d attempts: %w", method, url, attempt, err)
			}
			if serr := sleepCtx(ctx, backoff(base, attempt)); serr != nil {
				return nil, 0, serr
			}
			continue
		}
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		resp.Body.Close()
		if err != nil {
			return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, resp.StatusCode, nil
		}

		apiErr := &APIError{StatusCode: resp.StatusCode, Body: truncate(string(respBody), 2000)}
		if resp.StatusCode == http.StatusTooManyRequests {
			apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
		}
		if !retryableStatus(resp.StatusCode) {
			return respBody, resp.StatusCode, apiErr
		}
		if attempt > c.MaxRetries {
			return respBody, resp.StatusCode, fmt.Errorf("giving up after %d attempts: %w", attempt, apiErr)
		}
		wait := apiErr.RetryAfter
		if wait <= 0 {
			wait = backoff(base, attempt)
		}
		// NOTE: retrying a POST after a 5xx can duplicate the task when the
		// server processed it but the response was lost. ProofHub docs still
		// recommend retrying ("re-trying in some time should solve the problem"),
		// so we retry and let --retries 0 opt out.
		if serr := sleepCtx(ctx, wait); serr != nil {
			return nil, 0, serr
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- Tasks ---

// CreateTaskRequest mirrors POST v3/projects/{project}/todolists/{list}/tasks
type CreateTaskRequest struct {
	Title          string           `json:"title"`
	Description    string           `json:"description,omitempty"`
	StartDate      string           `json:"start_date,omitempty"` // YYYY-MM-DD
	DueDate        string           `json:"due_date,omitempty"`   // YYYY-MM-DD
	EstimatedHours *int             `json:"estimated_hours,omitempty"`
	EstimatedMins  *int             `json:"estimated_mins,omitempty"`
	Assigned       []int64          `json:"assigned,omitempty"`
	Labels         []int64          `json:"labels,omitempty"`
	Attachments    []TaskAttachment `json:"attachments,omitempty"`
	CustomFields   map[string]any   `json:"custom_fields,omitempty"`
}

type TaskAttachment struct {
	ID     string `json:"id"`
	Folder int    `json:"folder,omitempty"`
}

// Task is a subset of the Get task response (fields we care about).
type Task struct {
	ID              int64   `json:"id"`
	Ticket          string  `json:"ticket"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	StartDate       any     `json:"start_date"`
	DueDate         any     `json:"due_date"`
	EstimatedHours  *int    `json:"estimated_hours"`
	EstimatedMins   *int    `json:"estimated_mins"`
	LoggedHours     *int    `json:"logged_hours"`
	LoggedMins      *int    `json:"logged_mins"`
	PercentProgress int     `json:"percent_progress"`
	Completed       bool    `json:"completed"`
	Assigned        []int64 `json:"assigned"`
	Labels          []int64 `json:"labels"`
	Comments        int     `json:"comments"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	Project         struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	List struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"list"`
	Creator struct {
		ID int64 `json:"id"`
	} `json:"creator"`
	Workflow struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"workflow"`
	Stage        *Stage        `json:"stage"`
	CustomFields []CustomField `json:"custom_fields"`
}

// CustomField mirrors ProofHub custom field value in task detail.
type CustomField struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// Stage is a workflow stage (e.g. New, In Progress, Done).
// Single-task responses use "name", the stages list uses "title".
type Stage struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Title string `json:"title"`
}

// DisplayName returns the stage's human-readable name.
func (s Stage) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Title
}

// CreateTask creates a task inside project > todolist(list).
func (c *Client) CreateTask(ctx context.Context, projectID, listID string, req CreateTaskRequest) (*Task, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks", projectID, listID)
	b, _, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("decode create task response: %w (body: %s)", err, truncate(string(b), 1000))
	}
	return &t, nil
}

// GetTask returns a single task: GET v3/projects/{p}/todolists/{l}/tasks/{id}
func (c *Client) GetTask(ctx context.Context, projectID, listID, taskID string) (*Task, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("decode get task response: %w", err)
	}
	return &t, nil
}

// UpdateTaskRequest mirrors PUT v3/projects/{project}/todolists/{list}/tasks/{id}.
// Pointer fields distinguish "unset" from zero values; nil slices are omitted.
// PercentProgress, Logged* mirror ProofHub task fields (see sections/tasks.md).
type UpdateTaskRequest struct {
	Title           *string `json:"title,omitempty"`
	Description     *string `json:"description,omitempty"`
	StartDate       *string `json:"start_date,omitempty"` // YYYY-MM-DD
	DueDate         *string `json:"due_date,omitempty"`   // YYYY-MM-DD
	EstimatedHours  *int    `json:"estimated_hours,omitempty"`
	EstimatedMins   *int    `json:"estimated_mins,omitempty"`
	LoggedHours     *int    `json:"logged_hours,omitempty"`
	LoggedMins      *int    `json:"logged_mins,omitempty"`
	PercentProgress *int    `json:"percent_progress,omitempty"` // 0-100
	Assigned        []int64 `json:"assigned,omitempty"`
	Labels          []int64 `json:"labels,omitempty"`
	Completed       *bool   `json:"completed,omitempty"`
	StageID         *int64  `json:"stage,omitempty"`
}

// UpdateTask updates a task from the parameters passed.
func (c *Client) UpdateTask(
	ctx context.Context, projectID, listID, taskID string, req UpdateTaskRequest,
) (*Task, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodPut, path, req)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("decode update task response: %w", err)
	}
	return &t, nil
}

// DeleteTask: DELETE v3/projects/{p}/todolists/{l}/tasks/{id}
func (c *Client) DeleteTask(ctx context.Context, projectID, listID, taskID string) error {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s", projectID, listID, taskID)
	_, _, err := c.do(ctx, http.MethodDelete, path, nil)
	return err
}

// CopyTaskRequest mirrors POST v3/projects/{p}/todolists/{l}/tasks/{id} with copy_task.
type CopyTaskRequest struct {
	Title            string `json:"title,omitempty"`
	Project          *int64 `json:"project,omitempty"`
	ListID           *int64 `json:"list_id,omitempty"`
	Stage            *int64 `json:"stage,omitempty"`
	CopyAssignees    *bool  `json:"copy_assignees,omitempty"`
	CopyCustomFields *bool  `json:"copy_custom_fields,omitempty"`
	CopyDates        *bool  `json:"copy_dates,omitempty"`
	CopyComments     *bool  `json:"copy_comments,omitempty"`
	CopyTask         int64  `json:"copy_task"`
}

// CopyTask: POST v3/projects/{p}/todolists/{l}/tasks/{id} (duplicate)
func (c *Client) CopyTask(ctx context.Context, projectID, listID, taskID string, req CopyTaskRequest) (*Task, error) {
	if req.CopyTask == 0 {
		if n, err := strconv.ParseInt(taskID, 10, 64); err == nil {
			req.CopyTask = n
		}
	}
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("decode copy task response: %w", err)
	}
	return &t, nil
}

// MoveTaskRequest mirrors PUT v3/projects/{p}/todolists/{l}/tasks/{id} with move_task.
type MoveTaskRequest struct {
	Title            *string `json:"title,omitempty"`
	Project          *int64  `json:"project,omitempty"`
	ListID           *int64  `json:"list_id,omitempty"`
	Stage            *int64  `json:"stage,omitempty"`
	MovePeople       *bool   `json:"move_people,omitempty"`
	CopyAssignees    *bool   `json:"copy_assignees,omitempty"`
	CopyCustomFields *bool   `json:"copy_custom_fields,omitempty"`
	MoveDates        *bool   `json:"move_dates,omitempty"`
	ProofComment     *bool   `json:"proof_comment,omitempty"`
	CopyComments     *bool   `json:"copy_comments,omitempty"`
	Completed        *bool   `json:"completed,omitempty"`
	MoveTask         bool    `json:"move_task"`
	ID               *int64  `json:"id,omitempty"`
}

// MoveTask: PUT v3/projects/{p}/todolists/{l}/tasks/{id} (move)
func (c *Client) MoveTask(ctx context.Context, projectID, listID, taskID string, req MoveTaskRequest) (*Task, error) {
	req.MoveTask = true
	if req.ID == nil {
		if n, err := strconv.ParseInt(taskID, 10, 64); err == nil {
			req.ID = &n
		}
	}
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodPut, path, req)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("decode move task response: %w", err)
	}
	return &t, nil
}

// ListStages returns workflow stages: GET v3/workflows/{workflow}/stages.
// Undocumented in https://github.com/ProofHub/api_v3 but live.
func (c *Client) ListStages(ctx context.Context, workflowID string) ([]Stage, error) {
	path := fmt.Sprintf("/workflows/%s/stages", workflowID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var stages []Stage
	if err := json.Unmarshal(b, &stages); err != nil {
		return nil, fmt.Errorf("decode list stages response: %w", err)
	}
	return stages, nil
}

// ResolveStageID matches a stage name (case-insensitive) to its ID.
func (c *Client) ResolveStageID(ctx context.Context, workflowID, name string) (int64, error) {
	stages, err := c.ListStages(ctx, workflowID)
	if err != nil {
		return 0, err
	}
	for _, s := range stages {
		if strings.EqualFold(s.DisplayName(), name) {
			return s.ID, nil
		}
	}
	names := make([]string, 0, len(stages))
	for _, s := range stages {
		names = append(names, s.DisplayName())
	}
	return 0, fmt.Errorf("unknown stage %q (available: %s)", name, strings.Join(names, ", "))
}

// ListTasks returns tasks for a todolist: GET v3/projects/{p}/todolists/{l}/tasks
func (c *Client) ListTasks(ctx context.Context, projectID, listID string) ([]Task, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks", projectID, listID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	if err := json.Unmarshal(b, &tasks); err != nil {
		return nil, fmt.Errorf("decode list tasks response: %w", err)
	}
	return tasks, nil
}

// Subtask is same shape as Task (ProofHub returns task-like JSON for subtasks).
type Subtask = Task

// CreateSubtaskRequest mirrors POST .../tasks/{t}/subtasks
type CreateSubtaskRequest struct {
	Title          string           `json:"title"`
	Description    string           `json:"description,omitempty"`
	StartDate      string           `json:"start_date,omitempty"`
	DueDate        string           `json:"due_date,omitempty"`
	EstimatedHours *int             `json:"estimated_hours,omitempty"`
	EstimatedMins  *int             `json:"estimated_mins,omitempty"`
	Assigned       []int64          `json:"assigned,omitempty"`
	Labels         []int64          `json:"labels,omitempty"`
	Attachments    []TaskAttachment `json:"attachments,omitempty"`
}

// UpdateSubtaskRequest mirrors PUT .../subtasks/{sid}
type UpdateSubtaskRequest struct {
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	StartDate      *string `json:"start_date,omitempty"`
	DueDate        *string `json:"due_date,omitempty"`
	EstimatedHours *int    `json:"estimated_hours,omitempty"`
	EstimatedMins  *int    `json:"estimated_mins,omitempty"`
	Assigned       []int64 `json:"assigned,omitempty"`
	Labels         []int64 `json:"labels,omitempty"`
	Completed      *bool   `json:"completed,omitempty"`
}

// ListSubtasks: GET v3/projects/{p}/todolists/{l}/tasks/{t}/subtasks
func (c *Client) ListSubtasks(ctx context.Context, projectID, listID, taskID string) ([]Subtask, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/subtasks", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var subs []Subtask
	if err := json.Unmarshal(b, &subs); err != nil {
		return nil, fmt.Errorf("decode list subtasks response: %w", err)
	}
	return subs, nil
}

// GetSubtask: GET v3/projects/{p}/todolists/{l}/tasks/{t}/subtasks/{sid}
func (c *Client) GetSubtask(ctx context.Context, projectID, listID, taskID, subtaskID string) (*Subtask, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/subtasks/%s", projectID, listID, taskID, subtaskID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var s Subtask
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("decode get subtask response: %w", err)
	}
	return &s, nil
}

// CreateSubtask: POST v3/projects/{p}/todolists/{l}/tasks/{t}/subtasks
func (c *Client) CreateSubtask(ctx context.Context, projectID, listID, taskID string, req CreateSubtaskRequest) (*Subtask, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/subtasks", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	var s Subtask
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("decode create subtask response: %w", err)
	}
	return &s, nil
}

// UpdateSubtask: PUT v3/projects/{p}/todolists/{l}/tasks/{t}/subtasks/{sid}
func (c *Client) UpdateSubtask(ctx context.Context, projectID, listID, taskID, subtaskID string, req UpdateSubtaskRequest) (*Subtask, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/subtasks/%s", projectID, listID, taskID, subtaskID)
	b, _, err := c.do(ctx, http.MethodPut, path, req)
	if err != nil {
		return nil, err
	}
	var s Subtask
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("decode update subtask response: %w", err)
	}
	return &s, nil
}

// DeleteSubtask: DELETE v3/projects/{p}/todolists/{l}/tasks/{t}/subtasks/{sid}
func (c *Client) DeleteSubtask(ctx context.Context, projectID, listID, taskID, subtaskID string) error {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/subtasks/%s", projectID, listID, taskID, subtaskID)
	_, _, err := c.do(ctx, http.MethodDelete, path, nil)
	return err
}

// TaskHistoryEntry mirrors GET .../tasks/{t}/history
type TaskHistoryEntry struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Activity string `json:"activity"`
	Date     string `json:"date"`
	Action   string `json:"action"`
}

// TaskHistoryDetail mirrors GET .../history/{hid}
type TaskHistoryDetail struct {
	Content string `json:"content"`
}

// ListTaskHistory: GET v3/projects/{p}/todolists/{l}/tasks/{t}/history
func (c *Client) ListTaskHistory(ctx context.Context, projectID, listID, taskID string) ([]TaskHistoryEntry, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/history", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var h []TaskHistoryEntry
	if err := json.Unmarshal(b, &h); err != nil {
		return nil, fmt.Errorf("decode list task history response: %w", err)
	}
	return h, nil
}

// GetTaskHistoryDetail: GET v3/projects/{p}/todolists/{l}/tasks/{t}/history/{hid}
func (c *Client) GetTaskHistoryDetail(ctx context.Context, projectID, listID, taskID, historyID string) (*TaskHistoryDetail, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/history/%s", projectID, listID, taskID, historyID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var d TaskHistoryDetail
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("decode get task history detail response: %w", err)
	}
	return &d, nil
}

// Todolist mirrors GET todolist response (subset). Expanded for Get todolist detail.
type Todolist struct {
	ID                 int64   `json:"id"`
	Title              string  `json:"title"`
	Private            *bool   `json:"private,omitempty"`
	Archived           *bool   `json:"archived,omitempty"`
	CompletedCount     *int    `json:"completed_count,omitempty"`
	RemainingCount     *int    `json:"remaining_count,omitempty"`
	TimeTracking       *bool   `json:"time_tracking,omitempty"`
	ShowInGantt        *bool   `json:"show_in_gantt,omitempty"`
	UpdatedAt          string  `json:"updated_at,omitempty"`
	CreatedAt          string  `json:"created_at,omitempty"`
	Assigned           []int64 `json:"assigned,omitempty"`
	AssociateMilestone *bool   `json:"associate_milestone,omitempty"`
	ByMe               *bool   `json:"by_me,omitempty"`
	ReplyEmail         string  `json:"reply_email,omitempty"`
	Project            struct {
		ID int64 `json:"id"`
	} `json:"project,omitempty"`
	Timesheet *struct {
		ID *int64 `json:"id"`
	} `json:"timesheet,omitempty"`
	Milestone *struct {
		ID *int64 `json:"id"`
	} `json:"milestone,omitempty"`
	Creator *struct {
		ID int64 `json:"id"`
	} `json:"creator,omitempty"`
	Workflow *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"workflow,omitempty"`
	CustomFields []CustomField `json:"custom_fields,omitempty"`
}

// ListTodolists: GET v3/projects/{p}/todolists
func (c *Client) ListTodolists(ctx context.Context, projectID string) ([]Todolist, error) {
	path := fmt.Sprintf("/projects/%s/todolists", projectID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var lists []Todolist
	if err := json.Unmarshal(b, &lists); err != nil {
		return nil, fmt.Errorf("decode list todolists response: %w", err)
	}
	return lists, nil
}

// GetTodolist: GET v3/projects/{p}/todolists/{id}
func (c *Client) GetTodolist(ctx context.Context, projectID, todolistID string) (*Todolist, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s", projectID, todolistID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var tl Todolist
	if err := json.Unmarshal(b, &tl); err != nil {
		return nil, fmt.Errorf("decode get todolist response: %w", err)
	}
	return &tl, nil
}

// Person mirrors people API (subset).
type Person struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// ListPeople: GET v3/people
func (c *Client) ListPeople(ctx context.Context) ([]Person, error) {
	b, _, err := c.do(ctx, http.MethodGet, "/people", nil)
	if err != nil {
		return nil, err
	}
	var people []Person
	if err := json.Unmarshal(b, &people); err != nil {
		return nil, fmt.Errorf("decode people response: %w", err)
	}
	return people, nil
}

// Label mirrors GET labels (subset).
type Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// ListLabels: GET v3/labels
func (c *Client) ListLabels(ctx context.Context) ([]Label, error) {
	b, _, err := c.do(ctx, http.MethodGet, "/labels", nil)
	if err != nil {
		return nil, err
	}
	var labels []Label
	if err := json.Unmarshal(b, &labels); err != nil {
		return nil, fmt.Errorf("decode labels response: %w", err)
	}
	return labels, nil
}

// GetLabel: GET v3/labels/{id}
func (c *Client) GetLabel(ctx context.Context, labelID string) (*Label, error) {
	path := fmt.Sprintf("/labels/%s", labelID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var l Label
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("decode get label response: %w", err)
	}
	return &l, nil
}

// Comment mirrors ProofHub task comment (sections/tasks.md).
type Comment struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	Creator     struct {
		ID int64 `json:"id"`
	} `json:"creator"`
	Task struct {
		ID int64 `json:"id"`
	} `json:"task"`
	Project struct {
		ID int64 `json:"id"`
	} `json:"project"`
	List struct {
		ID int64 `json:"id"`
	} `json:"list"`
}

// ListComments: GET v3/projects/{p}/todolists/{l}/tasks/{t}/comments
func (c *Client) ListComments(ctx context.Context, projectID, listID, taskID string) ([]Comment, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/comments", projectID, listID, taskID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var comments []Comment
	if err := json.Unmarshal(b, &comments); err != nil {
		return nil, fmt.Errorf("decode list comments response: %w", err)
	}
	return comments, nil
}

// GetComment: GET v3/projects/{p}/todolists/{l}/tasks/{t}/comments/{c}
func (c *Client) GetComment(ctx context.Context, projectID, listID, taskID, commentID string) (*Comment, error) {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/comments/%s", projectID, listID, taskID, commentID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var cc Comment
	if err := json.Unmarshal(b, &cc); err != nil {
		return nil, fmt.Errorf("decode get comment response: %w", err)
	}
	return &cc, nil
}

// CreateComment: POST v3/projects/{p}/todolists/{l}/tasks/{t}/comments
func (c *Client) CreateComment(ctx context.Context, projectID, listID, taskID string, description string) (*Comment, error) {
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("comment description is required")
	}
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/comments", projectID, listID, taskID)
	body := map[string]string{"description": description}
	b, _, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	var cc Comment
	if err := json.Unmarshal(b, &cc); err != nil {
		return nil, fmt.Errorf("decode create comment response: %w", err)
	}
	return &cc, nil
}

// UpdateComment: PUT v3/projects/{p}/todolists/{l}/tasks/{t}/comments/{c}
func (c *Client) UpdateComment(ctx context.Context, projectID, listID, taskID, commentID string, description string) (*Comment, error) {
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("comment description is required")
	}
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/comments/%s", projectID, listID, taskID, commentID)
	body := map[string]string{"description": description}
	b, _, err := c.do(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	var cc Comment
	if err := json.Unmarshal(b, &cc); err != nil {
		return nil, fmt.Errorf("decode update comment response: %w", err)
	}
	return &cc, nil
}

// DeleteComment: DELETE v3/projects/{p}/todolists/{l}/tasks/{t}/comments/{c}
func (c *Client) DeleteComment(ctx context.Context, projectID, listID, taskID, commentID string) error {
	path := fmt.Sprintf("/projects/%s/todolists/%s/tasks/%s/comments/%s", projectID, listID, taskID, commentID)
	_, _, err := c.do(ctx, http.MethodDelete, path, nil)
	return err
}

// Timesheet mirrors v3/projects/{p}/timesheets
type Timesheet struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	EstimatedHours *int   `json:"estimated_hours"`
	EstimatedMins  *int   `json:"estimated_mins"`
	LoggedHours    *int   `json:"logged_hours"`
	LoggedMins     *int   `json:"logged_mins"`
	Project        struct {
		ID int64 `json:"id"`
	} `json:"project"`
	Assigned []int64 `json:"assigned"`
	Private  bool    `json:"private"`
	Archived bool    `json:"archived"`
}

// ListTimesheets: GET v3/projects/{p}/timesheets
func (c *Client) ListTimesheets(ctx context.Context, projectID string) ([]Timesheet, error) {
	path := fmt.Sprintf("/projects/%s/timesheets", projectID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var ts []Timesheet
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, fmt.Errorf("decode list timesheets response: %w", err)
	}
	return ts, nil
}

// GetTimesheet: GET v3/projects/{p}/timesheets/{id}
func (c *Client) GetTimesheet(ctx context.Context, projectID, timesheetID string) (*Timesheet, error) {
	path := fmt.Sprintf("/projects/%s/timesheets/%s", projectID, timesheetID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var t Timesheet
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("decode get timesheet response: %w", err)
	}
	return &t, nil
}

// TimeEntry mirrors v3/projects/{p}/timesheets/{ts}/time
type TimeEntry struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Date        string `json:"date"`
	LoggedHours int    `json:"logged_hours"`
	LoggedMins  int    `json:"logged_mins"`
	Status      string `json:"status"`
	Project     struct {
		ID int64 `json:"id"`
	} `json:"project"`
	Creator struct {
		ID int64 `json:"id"`
	} `json:"creator"`
	Task *struct {
		ID int64 `json:"id"`
	} `json:"task"`
	Timesheet struct {
		ID int64 `json:"id"`
	} `json:"timesheet"`
}

// CreateTimeEntryRequest mirrors POST v3/projects/{p}/timesheets/{ts}/time
type CreateTimeEntryRequest struct {
	Project     int64  `json:"project"`
	TimesheetID int64  `json:"timesheet_id"`
	Date        string `json:"date"` // YYYY-MM-DD
	LoggedHours *int   `json:"logged_hours,omitempty"`
	LoggedMins  *int   `json:"logged_mins,omitempty"`
	Status      string `json:"status,omitempty"` // billable/non-billable/none
	Description string `json:"description,omitempty"`
	TaskID      *int64 `json:"task_id,omitempty"`
	ListID      *int64 `json:"list_id,omitempty"`
}

// ListTimeEntries: GET v3/projects/{p}/timesheets/{ts}/time
func (c *Client) ListTimeEntries(ctx context.Context, projectID, timesheetID string) ([]TimeEntry, error) {
	path := fmt.Sprintf("/projects/%s/timesheets/%s/time", projectID, timesheetID)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var entries []TimeEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("decode list time entries response: %w", err)
	}
	return entries, nil
}

// CreateTimeEntry: POST v3/projects/{p}/timesheets/{ts}/time
func (c *Client) CreateTimeEntry(ctx context.Context, projectID, timesheetID string, req CreateTimeEntryRequest) (*TimeEntry, error) {
	if req.Project == 0 || req.TimesheetID == 0 {
		return nil, fmt.Errorf("project and timesheet_id are required")
	}
	if req.LoggedHours == nil && req.LoggedMins == nil {
		return nil, fmt.Errorf("one of logged_hours or logged_mins is required")
	}
	path := fmt.Sprintf("/projects/%s/timesheets/%s/time", projectID, timesheetID)
	b, _, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	var e TimeEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("decode create time entry response: %w", err)
	}
	return &e, nil
}

// ParseTarget extracts project and list IDs from forms like:
//
//	"8213786200" + "271478716253"
//	"project-8213786200/list-271478716253"
//	"project-8213786200" (list empty)
//	full ProofHub URL containing "project-.../list-..."
func ParseTarget(projectFlag, listFlag, targetFlag string) (projectID, listID string) {
	projectID = stripPrefixDigits(projectFlag, "project-")
	listID = stripPrefixDigits(listFlag, "list-")
	if targetFlag != "" {
		p, l := parseTargetString(targetFlag)
		if p != "" {
			projectID = p
		}
		if l != "" {
			listID = l
		}
	}
	// Also allow projectFlag itself to contain "project-x/list-y"
	hasSlash := strings.Contains(projectFlag, "/")
	hasBoth := strings.Contains(projectFlag, "project-") && strings.Contains(projectFlag, "list-")
	if hasSlash || hasBoth {
		p, l := parseTargetString(projectFlag)
		if p != "" {
			projectID = p
		}
		if l != "" {
			listID = l
		}
	}
	return projectID, listID
}

func stripPrefixDigits(s, prefix string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// If value is like "project-123", return "123".
	if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
		return s[len(prefix):]
	}
	return s
}

func parseTargetString(s string) (projectID, listID string) {
	s = strings.TrimSpace(s)
	// Normalize separators: allow full URLs, e.g. https://xxx.proofhub.com/.../project-1/list-2
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == '?' || r == '&' || r == ' ' || r == ','
	})
	for _, p := range parts {
		lp := strings.ToLower(p)
		if strings.HasPrefix(lp, "project-") {
			projectID = p[len("project-"):]
		} else if strings.HasPrefix(lp, "list-") {
			listID = p[len("list-"):]
		}
	}
	return digitsOnly(projectID), digitsOnly(listID)
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}
