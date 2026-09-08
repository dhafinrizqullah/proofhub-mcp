package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// tasks.go provides Task-related types and API methods.

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

