// Package gi implements a minimal Ghost Inspector API v1 client with
// retry/backoff and tolerant JSON decoding (the API emits raw control
// characters inside JSON strings, which encoding/json rejects).
package gi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.ghostinspector.com/v1"
	maxAttempts    = 6
	pageSize       = 100
)

// Client is a Ghost Inspector API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a client authenticated with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// envelope is the standard Ghost Inspector response wrapper.
type envelope struct {
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

// Error is an API-level error (code != SUCCESS).
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("ghost inspector API error: code=%s", e.Code)
	}
	return fmt.Sprintf("ghost inspector API error: code=%s message=%s", e.Code, e.Message)
}

// NotFound reports whether the error means the object no longer exists.
func NotFound(err error) bool {
	if err == nil {
		return false
	}
	var giErr *Error
	if ok := errorAs(err, &giErr); ok {
		if giErr.Code == "HTTP_404" {
			return true
		}
		return giErr.Code == "ERROR" && (strings.Contains(strings.ToLower(giErr.Message), "not found") ||
			strings.Contains(strings.ToLower(giErr.Message), "does not exist") ||
			strings.Contains(strings.ToLower(giErr.Message), "invalid"))
	}
	return false
}

func errorAs(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// sanitize strips bytes that are invalid inside JSON strings, matching the
// defensive filtering every Ghost Inspector API consumer ends up writing:
// keep tab (0x09) and newline (0x0A), drop everything else below 0x20.
func sanitize(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c < 0x20 && c != 0x09 && c != 0x0A {
			continue
		}
		out = append(out, c)
	}
	return out
}

// do executes one HTTP request against the API, with retry/backoff on
// rate limiting (429), transient 5xx responses, and transport errors.
// It returns the decoded `data` field of the envelope.
func (c *Client) do(ctx context.Context, method, path string, payload interface{}) (json.RawMessage, error) {
	var bodyBytes []byte
	var err error
	if payload != nil {
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	reqURL := fmt.Sprintf("%s%s%sapiKey=%s", c.baseURL, path, sep, url.QueryEscape(c.apiKey))

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			backoff += time.Duration(rand.Int63n(int64(time.Second)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		var body io.Reader
		if bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request %s %s: %w", method, path, err)
			continue // transport error: retry
		}

		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response %s %s: %w", method, path, readErr)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("request %s %s: HTTP %d", method, path, resp.StatusCode)
			continue // retry
		}
		if resp.StatusCode >= 400 {
			return nil, &Error{Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(sanitize(raw))}
		}

		clean := sanitize(raw)

		// Most endpoints wrap payloads in {code, data}; a few (test export)
		// return the bare document. Detect the envelope by its "code" field.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(clean, &probe); err != nil {
			return nil, fmt.Errorf("decode response %s %s: %w", method, path, err)
		}
		if _, enveloped := probe["code"]; !enveloped {
			return json.RawMessage(clean), nil
		}

		var env envelope
		if err := json.Unmarshal(clean, &env); err != nil {
			return nil, fmt.Errorf("decode response %s %s: %w", method, path, err)
		}
		if env.Code != "SUCCESS" {
			return nil, &Error{Code: env.Code, Message: env.Message}
		}
		return env.Data, nil
	}
	return nil, lastErr
}

// list fetches all pages of a list endpoint and concatenates the data arrays.
func (c *Client) list(ctx context.Context, path string) ([]json.RawMessage, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var all []json.RawMessage
	for offset := 0; ; offset += pageSize {
		data, err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s%scount=%d&offset=%d", path, sep, pageSize, offset), nil)
		if err != nil {
			return nil, err
		}
		var page []json.RawMessage
		if len(data) > 0 {
			if err := json.Unmarshal(data, &page); err != nil {
				return nil, fmt.Errorf("decode list page: %w", err)
			}
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
	}
	return all, nil
}
