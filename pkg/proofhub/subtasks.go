package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// subtasks.go provides Subtask types and API methods.

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

