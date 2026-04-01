package atlassian

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	url := config.URL
	if url == "" {
		url = os.Getenv("ATLASSIAN_URL")
	}

	user := config.User
	if user == "" {
		user = os.Getenv("ATLASSIAN_USER")
	}

	token := config.Token
	if token == "" {
		token = os.Getenv("ATLASSIAN_TOKEN")
	}

	if url == "" {
		return nil, fmt.Errorf("atlassian URL is required (set url in provider config or ATLASSIAN_URL env var)")
	}
	if user == "" {
		return nil, fmt.Errorf("atlassian user is required (set user in provider config or ATLASSIAN_USER env var)")
	}
	if token == "" {
		return nil, fmt.Errorf("atlassian token is required (set token in provider config or ATLASSIAN_TOKEN env var)")
	}

	baseURL := strings.TrimRight(url, "/")

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

// newRequest creates a new HTTP request with authentication and standard headers.
func (c *Client) newRequest(method, path string, body *bytes.Reader) (*http.Request, error) {
	url := c.baseURL + path

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, body)
	} else {
		req, err = http.NewRequest(method, url, nil)
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
