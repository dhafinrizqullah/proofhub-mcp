package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// people.go provides Person types and global People API methods.

// Person mirrors ProofHub people API (global, not scoped to project/todolist).
type Person struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// ListPeople: GET v3/people (global, not scoped to project/todolist)
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

// GetPerson: GET v3/people/{id} (global)
func (c *Client) GetPerson(ctx context.Context, id string) (*Person, error) {
	path := fmt.Sprintf("/people/%s", id)
	b, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var p Person
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("decode get person response: %w", err)
	}
	return &p, nil
}
