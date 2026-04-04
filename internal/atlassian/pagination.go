package atlassian

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// MaxPages is the maximum number of pages GetAllPages will fetch.
// At 50 items per page this covers 5000 items — far beyond any real Jira instance.
// An error is returned if this limit is exceeded, preventing infinite loops from
// misbehaving APIs that always return isLast=false.
const MaxPages = 100

// GetAllPages fetches all pages from a Jira paginated endpoint.
// The path should be the API path without pagination query parameters.
// Results are returned as raw JSON messages that the caller can unmarshal.
func (c *Client) GetAllPages(ctx context.Context, path string) ([]json.RawMessage, error) {
	allValues := make([]json.RawMessage, 0)
	startAt := 0
	maxResults := 50

	for page := 0; page < MaxPages; page++ {
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
			return allValues, nil
		}

		// Guard against APIs that don't set isLast correctly:
		// if we got fewer values than the page capacity, we've reached the end.
		if page.MaxResults > 0 && len(page.Values) < page.MaxResults {
			return allValues, nil
		}

		startAt += len(page.Values)
	}

	return nil, fmt.Errorf("pagination exceeded %d pages for %s", MaxPages, path)
}

// CursorPageResponse is the cursor-based pagination response used by Confluence v2 APIs.
type CursorPageResponse struct {
	Results []json.RawMessage `json:"results"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}

// GetAllPagesCursor fetches all pages from a Confluence v2 cursor-paginated endpoint.
// The path should be the API path (relative). Results are returned as raw JSON messages.
func (c *Client) GetAllPagesCursor(ctx context.Context, path string) ([]json.RawMessage, error) {
	allResults := make([]json.RawMessage, 0)
	currentPath := path

	for {
		var page CursorPageResponse
		if err := c.Get(ctx, currentPath, &page); err != nil {
			return nil, fmt.Errorf("fetching cursor page at %s: %w", currentPath, err)
		}

		allResults = append(allResults, page.Results...)

		if page.Links.Next == "" {
			break
		}

		next := page.Links.Next
		// Handle absolute URLs: strip base URL to get relative path.
		// Reject URLs pointing to a different host to avoid malformed requests.
		if strings.HasPrefix(next, "http") {
			if strings.HasPrefix(next, c.baseURL) {
				next = next[len(c.baseURL):]
			} else {
				return nil, fmt.Errorf("cursor pagination: unexpected absolute URL from different host: %s", next)
			}
		}
		currentPath = next
	}

	return allResults, nil
}

// GetAllPagesCursorWithStatus is like GetAllPagesCursor but treats a 404 on the first
// request as (nil, 404, nil) rather than an error. Callers can use this to detect when
// the parent resource (e.g. a space) has been deleted out-of-band.
func (c *Client) GetAllPagesCursorWithStatus(ctx context.Context, path string) ([]json.RawMessage, int, error) {
	allResults := make([]json.RawMessage, 0)
	currentPath := path
	firstPage := true

	for {
		var page CursorPageResponse
		if firstPage {
			statusCode, err := c.GetWithStatus(ctx, currentPath, &page)
			if err != nil {
				return nil, statusCode, err
			}
			if statusCode == http.StatusNotFound {
				return nil, http.StatusNotFound, nil
			}
			firstPage = false
		} else {
			if err := c.Get(ctx, currentPath, &page); err != nil {
				return nil, 0, fmt.Errorf("fetching cursor page at %s: %w", currentPath, err)
			}
		}

		allResults = append(allResults, page.Results...)

		if page.Links.Next == "" {
			break
		}

		next := page.Links.Next
		if strings.HasPrefix(next, "http") {
			if strings.HasPrefix(next, c.baseURL) {
				next = next[len(c.baseURL):]
			} else {
				return nil, 0, fmt.Errorf("cursor pagination: unexpected absolute URL from different host: %s", next)
			}
		}
		currentPath = next
	}

	return allResults, http.StatusOK, nil
}
