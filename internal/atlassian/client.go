package atlassian

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries = 5
	baseDelay  = 1 * time.Second
	maxDelay   = 30 * time.Second
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

	return &Client{
		baseURL:    baseURL,
		user:       user,
		token:      token,
		version:    config.Version,
		httpClient: &http.Client{},
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

// newRequest creates a new HTTP request with authentication and standard headers.
func (c *Client) newRequest(method, path string, body *bytes.Reader) (*http.Request, error) {
	u := c.baseURL + path

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, u, body)
	} else {
		req, err = http.NewRequest(method, u, nil)
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
func (c *Client) Do(method, path string, body []byte) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Reset body reader position for retries
		if bodyReader != nil {
			bodyReader.Seek(0, io.SeekStart)
		}

		req, err := c.newRequest(method, path, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}

		// Rate limited
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("rate limited after %d retries", maxRetries)
			}
			delay := parseRetryAfter(resp.Header.Get("Retry-After"))
			time.Sleep(delay)
			continue
		}

		// Server error — retry with exponential backoff
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("server error (HTTP %d) after %d retries", resp.StatusCode, maxRetries)
			}
			delay := exponentialBackoff(attempt)
			time.Sleep(delay)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries", maxRetries)
}

// Get performs a GET request and decodes the JSON response into v.
func (c *Client) Get(path string, v interface{}) error {
	resp, err := c.Do("GET", path, nil)
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
func (c *Client) GetWithStatus(path string, v interface{}) (int, error) {
	resp, err := c.Do("GET", path, nil)
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
func (c *Client) Post(path string, body interface{}, v interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := c.Do("POST", path, jsonBody)
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
func (c *Client) Put(path string, body interface{}, v interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := c.Do("PUT", path, jsonBody)
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
func (c *Client) Patch(path string, body interface{}, v interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := c.Do("PATCH", path, jsonBody)
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
func (c *Client) Delete(path string) error {
	resp, err := c.Do("DELETE", path, nil)
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

// parseRetryAfter parses the Retry-After header value as seconds.
// Returns baseDelay if the header cannot be parsed.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return baseDelay
	}

	seconds, err := strconv.Atoi(header)
	if err != nil {
		return baseDelay
	}

	return time.Duration(seconds) * time.Second
}

// exponentialBackoff calculates the delay for a given retry attempt.
func exponentialBackoff(attempt int) time.Duration {
	delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
