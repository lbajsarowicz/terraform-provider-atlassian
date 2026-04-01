package atlassian

import (
	"fmt"
	"net/http"
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
func NewClient(config ClientConfig) (*Client, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("atlassian URL is required (set url in provider config or ATLASSIAN_URL env var)")
	}
	if config.User == "" {
		return nil, fmt.Errorf("atlassian user is required (set user in provider config or ATLASSIAN_USER env var)")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("atlassian token is required (set token in provider config or ATLASSIAN_TOKEN env var)")
	}

	baseURL := strings.TrimRight(config.URL, "/")

	return &Client{
		baseURL:    baseURL,
		user:       config.User,
		token:      config.Token,
		version:    config.Version,
		httpClient: &http.Client{},
	}, nil
}

// BaseURL returns the base URL of the Atlassian instance.
func (c *Client) BaseURL() string {
	return c.baseURL
}
