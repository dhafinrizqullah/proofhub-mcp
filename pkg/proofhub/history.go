package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// history.go provides task history types and API methods.

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

