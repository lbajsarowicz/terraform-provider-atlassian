package atlassian

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewClientWithEnvVars(t *testing.T) {
	t.Setenv("ATLASSIAN_URL", "https://test.atlassian.net")
	t.Setenv("ATLASSIAN_USER", "user@example.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	client, err := NewClient(ClientConfig{Version: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if client.BaseURL() != "https://test.atlassian.net" {
		t.Errorf("expected base URL %q, got %q", "https://test.atlassian.net", client.BaseURL())
	}
}

func TestNewClientExplicitOverridesEnv(t *testing.T) {
	t.Setenv("ATLASSIAN_URL", "https://env.atlassian.net")
	t.Setenv("ATLASSIAN_USER", "env@example.com")
	t.Setenv("ATLASSIAN_TOKEN", "env-token")

	client, err := NewClient(ClientConfig{
		URL:     "https://explicit.atlassian.net",
		User:    "explicit@example.com",
		Token:   "explicit-token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if client.BaseURL() != "https://explicit.atlassian.net" {
		t.Errorf("expected base URL %q, got %q", "https://explicit.atlassian.net", client.BaseURL())
	}
	if client.user != "explicit@example.com" {
		t.Errorf("expected user %q, got %q", "explicit@example.com", client.user)
	}
	if client.token != "explicit-token" {
		t.Errorf("expected token %q, got %q", "explicit-token", client.token)
	}
}

func TestNewClientMissingCredentials(t *testing.T) {
	// Clear env vars (t.Setenv auto-restores after test)
	t.Setenv("ATLASSIAN_URL", "")
	t.Setenv("ATLASSIAN_USER", "")
	t.Setenv("ATLASSIAN_TOKEN", "")

	_, err := NewClient(ClientConfig{})
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}

	_, err = NewClient(ClientConfig{URL: "https://test.atlassian.net"})
	if err == nil {
		t.Fatal("expected error for missing user")
	}

	_, err = NewClient(ClientConfig{URL: "https://test.atlassian.net", User: "user@example.com"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	client, err := NewClient(ClientConfig{
		URL:     "https://test.atlassian.net/",
		User:    "user@example.com",
		Token:   "token",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if client.BaseURL() != "https://test.atlassian.net" {
		t.Errorf("expected trailing slash to be trimmed, got %q", client.BaseURL())
	}
}

func TestNewRequestSetsHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "terraform-provider-atlassian/1.0.0" {
			t.Errorf("unexpected User-Agent: %s", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected Accept: %s", r.Header.Get("Accept"))
		}

		user, token, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth to be set")
		}
		if user != "user@example.com" {
			t.Errorf("unexpected user: %s", user)
		}
		if token != "test-token" {
			t.Errorf("unexpected token: %s", token)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		URL:     server.URL,
		User:    "user@example.com",
		Token:   "test-token",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	req, err := client.newRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error creating request: %s", err)
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error doing request: %s", err)
	}
	defer resp.Body.Close()
}

func TestRateLimitRetry(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
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

	resp, err := client.Do("GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer resp.Body.Close()

	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetryPreservesPostBody(t *testing.T) {
	var bodies []string
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(bodyBytes))

		attempt := attempts.Add(1)
		if attempt == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
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

	body := []byte(`{"name":"test-project"}`)
	resp, err := client.Do("POST", "/test", body)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer resp.Body.Close()

	if len(bodies) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("body not preserved across retries: %q vs %q", bodies[0], bodies[1])
	}
	if bodies[0] != `{"name":"test-project"}` {
		t.Errorf("unexpected body: %s", bodies[0])
	}
}

func TestQueryEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello+world"},
		{"foo@bar.com", "foo%40bar.com"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		result := QueryEscape(tt.input)
		if result != tt.expected {
			t.Errorf("QueryEscape(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetWithStatus404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
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

	var result map[string]interface{}
	status, err := client.GetWithStatus("/not-found", &result)
	if err != nil {
		t.Fatalf("expected nil error for 404, got: %s", err)
	}
	if status != 404 {
		t.Errorf("expected status 404, got %d", status)
	}
}
