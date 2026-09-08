package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// labels.go provides Label types and API methods.

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

