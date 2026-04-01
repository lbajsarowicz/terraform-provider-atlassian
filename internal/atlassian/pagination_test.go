package atlassian

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
