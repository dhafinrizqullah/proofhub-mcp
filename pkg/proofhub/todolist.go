package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// todolist.go provides Todolist types and API methods.

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
