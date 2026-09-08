package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// comments.go provides Comment types and API methods.

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

