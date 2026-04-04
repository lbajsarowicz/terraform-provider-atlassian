package atlassian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries           = 5
	baseDelay            = 1 * time.Second
	maxRetryDelay        = 30 * time.Second
	maxRetryAfterSeconds = 60
)

// Client is the Atlassian Cloud API client.
type Client struct {
	baseURL    string
	user       string
	token      string
	version    string
	httpClient *http.Client
}

// ClientConfig holds the configuration for creating a new Client.
type ClientConfig struct {
	URL     string
	User    string
	Token   string
	Version string
	// ResponseHeaderTimeout is how long to wait for response headers before
	// cancelling the request. Defaults to 30s. Set a shorter value in tests.
	ResponseHeaderTimeout time.Duration
}

// NewClient creates a new Atlassian API client.
// Explicit config values take precedence over environment variables.
func NewClient(config ClientConfig) (*Client, error) {
	u := config.URL
	if u == "" {
		u = os.Getenv("ATLASSIAN_URL")
	}

	user := config.User
	if user == "" {
		user = os.Getenv("ATLASSIAN_USER")
	}

	token := config.Token
	if token == "" {
		token = os.Getenv("ATLASSIAN_TOKEN")
	}

	if u == "" {
		return nil, fmt.Errorf("atlassian URL is required (set url in provider config or ATLASSIAN_URL env var)")
	}
	if user == "" {
		return nil, fmt.Errorf("atlassian user is required (set user in provider config or ATLASSIAN_USER env var)")
	}
	if token == "" {
		return nil, fmt.Errorf("atlassian token is required (set token in provider config or ATLASSIAN_TOKEN env var)")
	}

	baseURL := strings.TrimRight(u, "/")

	responseHeaderTimeout := 30 * time.Second
	if config.ResponseHeaderTimeout > 0 {
		responseHeaderTimeout = config.ResponseHeaderTimeout
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = responseHeaderTimeout

	return &Client{
		baseURL: baseURL,
		user:    user,
		token:   token,
		version: config.Version,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}, nil
}

// BaseURL returns the base URL of the Atlassian instance.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// QueryEscape escapes a string for use in URL query parameters.
func QueryEscape(s string) string {
	return url.QueryEscape(s)
}

// PathEscape escapes a string for use in URL path segments.
// Unlike QueryEscape, this encodes spaces as %20 instead of +.
func PathEscape(s string) string {
	return url.PathEscape(s)
}

// newRequest creates a new HTTP request with authentication and standard headers.
func (c *Client) newRequest(ctx context.Context, method, path string, body *bytes.Reader) (*http.Request, error) {
	u := c.baseURL + path

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, u, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, u, nil)
	}
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.user, c.token)
	req.Header.Set("User-Agent", fmt.Sprintf("terraform-provider-atlassian/%s", c.version))
	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// Do executes an HTTP request with retry logic for rate limits and server errors.
// The body parameter uses bytes.NewReader so it can be replayed on retries.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Reset body reader position for retries
		if bodyReader != nil {
			_, _ = bodyReader.Seek(0, io.SeekStart)
		}

		req, err := c.newRequest(ctx, method, path, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}

		// Rate limited
		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("rate limited after %d retries", maxRetries)
			}
			delay := parseRetryAfter(resp.Header.Get("Retry-After"))
			if err := sleepWithContext(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		// Server error — retry with exponential backoff
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("server error (HTTP %d) after %d retries", resp.StatusCode, maxRetries)
			}
			delay := exponentialBackoff(attempt)
			if err := sleepWithContext(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries", maxRetries)
}

// Get performs a GET request and decodes the JSON response into v.
func (c *Client) Get(ctx context.Context, path string, v interface{}) error {
	resp, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

// GetWithStatus performs a GET request and returns the status code.
// On 404, returns (404, nil) — the caller decides whether to remove state.
func (c *Client) GetWithStatus(ctx context.Context, path string, v interface{}) (int, error) {
	resp, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return http.StatusNotFound, nil
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("GET %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return resp.StatusCode, err
	}

	return resp.StatusCode, nil
}

// Post performs a POST request with a JSON body and decodes the response into v.
func (c *Client) Post(ctx context.Context, path string, body interface{}, v interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := c.Do(ctx, "POST", path, jsonBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}

	return nil
}

// Put performs a PUT request with a JSON body and decodes the response into v.
func (c *Client) Put(ctx context.Context, path string, body interface{}, v interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := c.Do(ctx, "PUT", path, jsonBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}

	return nil
}

// Patch performs a PATCH request with a JSON body and decodes the response into v.
func (c *Client) Patch(ctx context.Context, path string, body interface{}, v interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := c.Do(ctx, "PATCH", path, jsonBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}

	return nil
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	resp, err := c.Do(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// DeleteWithStatus performs a DELETE request and returns the HTTP status code.
// On 404, returns (404, nil) — the caller decides how to handle it.
func (c *Client) DeleteWithStatus(ctx context.Context, path string) (int, error) {
	resp, err := c.Do(ctx, "DELETE", path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return http.StatusNotFound, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("DELETE %s: unexpected status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	return resp.StatusCode, nil
}

// parseRetryAfter parses the Retry-After header value as seconds.
// Returns baseDelay if the header cannot be parsed.
// Caps the value at maxRetryAfterSeconds to prevent multi-minute sleeps.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return baseDelay
	}

	seconds, err := strconv.Atoi(header)
	if err != nil || seconds <= 0 {
		return baseDelay
	}

	if seconds > maxRetryAfterSeconds {
		seconds = maxRetryAfterSeconds
	}

	return time.Duration(seconds) * time.Second
}

// sleepWithContext sleeps for d or until ctx is cancelled, whichever comes first.
// Returns ctx.Err() if the context is cancelled before the sleep completes.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// exponentialBackoff calculates the delay for a given retry attempt with jitter
// to prevent thundering herd on rate limits.
func exponentialBackoff(attempt int) time.Duration {
	delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
	jitter := 0.5 + rand.Float64()*0.5 // 0.5 to 1.0
	delay = time.Duration(float64(delay) * 2 * jitter)
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	return delay
}
