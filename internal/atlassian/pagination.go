package atlassian

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PageResponse represents Jira's offset-based pagination response.
type PageResponse struct {
	StartAt    int               `json:"startAt"`
	MaxResults int               `json:"maxResults"`
	Total      int               `json:"total"`
	IsLast     bool              `json:"isLast"`
	Values     []json.RawMessage `json:"values"`
}

// GetAllPages fetches all pages from a Jira paginated endpoint.
// The path should be the API path without pagination query parameters.
// Results are returned as raw JSON messages that the caller can unmarshal.
func (c *Client) GetAllPages(ctx context.Context, path string) ([]json.RawMessage, error) {
	allValues := make([]json.RawMessage, 0)
	startAt := 0
	maxResults := 50

	for {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}

		paginatedPath := fmt.Sprintf("%s%sstartAt=%d&maxResults=%d", path, separator, startAt, maxResults)

		var page PageResponse
		if err := c.Get(ctx, paginatedPath, &page); err != nil {
			return nil, fmt.Errorf("fetching page at startAt=%d: %w", startAt, err)
		}

		allValues = append(allValues, page.Values...)

		if page.IsLast || len(page.Values) == 0 {
			break
		}

		startAt += len(page.Values)
	}

	return allValues, nil
}
