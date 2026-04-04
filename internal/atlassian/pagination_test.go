package atlassian

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGetAllPagesMultiplePages(t *testing.T) {
	// 3 items across 2 pages (maxResults=2 per page)
	items := []map[string]string{
		{"id": "1", "name": "first"},
		{"id": "2", "name": "second"},
		{"id": "3", "name": "third"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		startAt := 0
		fmt.Sscanf(query.Get("startAt"), "%d", &startAt)

		var pageItems []map[string]string
		isLast := false

		if startAt == 0 {
			pageItems = items[0:2]
		} else {
			pageItems = items[2:]
			isLast = true
		}

		values := make([]json.RawMessage, len(pageItems))
		for i, item := range pageItems {
			values[i], _ = json.Marshal(item)
		}

		resp := PageResponse{
			StartAt:    startAt,
			MaxResults: 2,
			Total:      3,
			IsLast:     isLast,
			Values:     values,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	var item map[string]string
	json.Unmarshal(results[2], &item)
	if item["name"] != "third" {
		t.Errorf("expected third item name %q, got %q", "third", item["name"])
	}
}

func TestGetAllPagesSinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := []json.RawMessage{
			json.RawMessage(`{"id":"1"}`),
		}

		resp := PageResponse{
			StartAt:    0,
			MaxResults: 50,
			Total:      1,
			IsLast:     true,
			Values:     values,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestGetAllPagesWithExistingQueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		// Verify the existing query param is preserved
		if query.Get("projectId") != "10001" {
			t.Errorf("expected projectId=10001, got %q", query.Get("projectId"))
		}

		// Verify pagination params are present
		if query.Get("startAt") == "" {
			t.Error("expected startAt query parameter")
		}

		resp := PageResponse{
			StartAt:    0,
			MaxResults: 50,
			Total:      0,
			IsLast:     true,
			Values:     []json.RawMessage{},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items?projectId=10001")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestGetAllPagesIsLastFalseButFewerThanMaxResults(t *testing.T) {
	// Reproduces the bug where Jira returns isLast=false even when there
	// are no more pages. Without the MaxResults guard this loops forever.
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 2 {
			t.Fatal("pagination did not terminate — infinite loop detected")
		}
		resp := PageResponse{
			StartAt:    0,
			MaxResults: 50,
			Total:      1,
			IsLast:     false, // API lies about isLast
			Values:     []json.RawMessage{json.RawMessage(`{"id":"1"}`)},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestGetAllPagesIsLastFalseMaxResultsEqualsValues(t *testing.T) {
	// Reproduces the real Jira bug: workflowscheme/project returns
	// isLast=false, maxResults=1, and 1 value — all three termination
	// conditions fail and the loop runs until MaxPages.
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if requestCount.Load() > 3 {
			t.Fatal("pagination did not terminate — infinite loop detected")
		}
		resp := PageResponse{
			StartAt:    0,
			MaxResults: 1,
			Total:      1,
			IsLast:     false, // API lies about isLast
			Values:     []json.RawMessage{json.RawMessage(`{"id":"1"}`)},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestGetAllPagesMaxResultsZeroStopsOnTotal(t *testing.T) {
	// API returns maxResults=0 (some Jira endpoints do this),
	// but Total is set correctly. Should still terminate.
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if requestCount.Load() > 3 {
			t.Fatal("pagination did not terminate — infinite loop detected")
		}
		resp := PageResponse{
			StartAt:    0,
			MaxResults: 0,
			Total:      2,
			IsLast:     false,
			Values:     []json.RawMessage{json.RawMessage(`{"id":"1"}`), json.RawMessage(`{"id":"2"}`)},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestGetAllPagesStartAtMismatchStops(t *testing.T) {
	// Server that ignores startAt and always returns the same page.
	// The startAt mismatch guard should terminate after 2 requests.
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		values := make([]json.RawMessage, 50)
		for i := range values {
			values[i] = json.RawMessage(`{"id":"1"}`)
		}
		resp := PageResponse{
			StartAt:    0, // always 0, ignoring our startAt param
			MaxResults: 50,
			IsLast:     false,
			Values:     values,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	results, err := client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// Should get 2 pages worth: first page (startAt=0 matches), second page (startAt=50 != 0, stop).
	if len(results) != 100 {
		t.Fatalf("expected 100 results (2 pages), got %d", len(results))
	}
	if requestCount.Load() != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount.Load())
	}
}

func TestGetAllPagesMaxPagesExceeded(t *testing.T) {
	// Server that echoes startAt correctly but never sets isLast=true,
	// simulating an API with genuinely infinite pages.
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		query := r.URL.Query()
		startAt := 0
		fmt.Sscanf(query.Get("startAt"), "%d", &startAt)

		values := make([]json.RawMessage, 50)
		for i := range values {
			values[i] = json.RawMessage(`{"id":"1"}`)
		}
		resp := PageResponse{
			StartAt:    startAt, // echo back correctly
			MaxResults: 50,
			IsLast:     false,
			Values:     values,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	_, err = client.GetAllPages(context.Background(), "/rest/api/3/items")
	if err == nil {
		t.Fatal("expected error when max pages exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("expected 'exceeded' in error, got: %v", err)
	}
	if requestCount.Load() != int32(MaxPages) {
		t.Errorf("expected %d requests, got %d", MaxPages, requestCount.Load())
	}
}
