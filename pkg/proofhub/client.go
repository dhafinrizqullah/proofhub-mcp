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

