package proofhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// timesheets.go provides Timesheet and TimeEntry types and API methods.

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

