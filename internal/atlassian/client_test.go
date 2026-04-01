package atlassian

import (
	"net/http"
	"net/http/httptest"
	"os"
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
	// Clear env vars
	os.Unsetenv("ATLASSIAN_URL")
	os.Unsetenv("ATLASSIAN_USER")
	os.Unsetenv("ATLASSIAN_TOKEN")

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
		// Verify headers
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
