package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func screenSchemeJSON(id int64, name, description string, screens map[string]int64) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
		"screens":     screens,
	}
}

// paginatedScreenSchemeResponse wraps a slice of screen schemes in the Jira paginated response shape.
func paginatedScreenSchemeResponse(schemes []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"values":     schemes,
		"startAt":    0,
		"maxResults": 50,
		"total":      len(schemes),
		"isLast":     true,
	}
}

// ---------------------------------------------------------------------------
// TestAccScreenSchemeResource_basic — create/read lifecycle
// ---------------------------------------------------------------------------

func TestAccScreenSchemeResource_basic(t *testing.T) {
	var mu sync.Mutex
	schemes := []map[string]interface{}{}
	var nextID atomic.Int64
	nextID.Store(10000)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// ---- Create screen scheme
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screenscheme":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			id := nextID.Add(1)

			screens := map[string]int64{}
			if s, ok := body["screens"].(map[string]interface{}); ok {
				for k, v := range s {
					switch val := v.(type) {
					case float64:
						screens[k] = int64(val)
					}
				}
			}
			scheme := screenSchemeJSON(id, body["name"].(string), fmt.Sprintf("%v", body["description"]), screens)
			mu.Lock()
			schemes = append(schemes, scheme)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": id}) //nolint:errcheck

		// ---- List screen schemes (paginated)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screenscheme":
			mu.Lock()
			all := make([]map[string]interface{}, len(schemes))
			copy(all, schemes)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(paginatedScreenSchemeResponse(all)) //nolint:errcheck

		// ---- Delete screen scheme
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screenscheme/"):
			schemeID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/screenscheme/")
			mu.Lock()
			newSchemes := []map[string]interface{}{}
			for _, s := range schemes {
				if fmt.Sprintf("%v", s["id"]) != schemeID {
					newSchemes = append(newSchemes, s)
				}
			}
			schemes = newSchemes
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "atlassian_jira_screen_scheme" "test" {
					name        = "Test Screen Scheme"
					description = "Initial description"
					screens = {
						default = "10001"
					}
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_scheme.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "name", "Test Screen Scheme"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "description", "Initial description"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "screens.default", "10001"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenSchemeResource_update — update name/description/screens
// ---------------------------------------------------------------------------

func TestAccScreenSchemeResource_update(t *testing.T) {
	var mu sync.Mutex
	schemes := []map[string]interface{}{}
	var nextID atomic.Int64
	nextID.Store(20000)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// ---- Create screen scheme
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screenscheme":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			id := nextID.Add(1)
			screens := map[string]int64{}
			if s, ok := body["screens"].(map[string]interface{}); ok {
				for k, v := range s {
					if val, ok := v.(float64); ok {
						screens[k] = int64(val)
					}
				}
			}
			desc := ""
			if d, ok := body["description"].(string); ok {
				desc = d
			}
			scheme := screenSchemeJSON(id, body["name"].(string), desc, screens)
			mu.Lock()
			schemes = append(schemes, scheme)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": id}) //nolint:errcheck

		// ---- Update screen scheme
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screenscheme/"):
			schemeID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/screenscheme/")
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			for i, s := range schemes {
				if fmt.Sprintf("%v", s["id"]) == schemeID {
					schemes[i]["name"] = body["name"]
					if d, ok := body["description"].(string); ok {
						schemes[i]["description"] = d
					}
					if screens, ok := body["screens"].(map[string]interface{}); ok {
						newScreens := map[string]int64{}
						for k, v := range screens {
							if val, ok := v.(float64); ok {
								newScreens[k] = int64(val)
							}
						}
						schemes[i]["screens"] = newScreens
					}
					break
				}
			}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// ---- List screen schemes (paginated)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screenscheme":
			mu.Lock()
			all := make([]map[string]interface{}, len(schemes))
			copy(all, schemes)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(paginatedScreenSchemeResponse(all)) //nolint:errcheck

		// ---- Delete screen scheme
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screenscheme/"):
			schemeID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/screenscheme/")
			mu.Lock()
			newSchemes := []map[string]interface{}{}
			for _, s := range schemes {
				if fmt.Sprintf("%v", s["id"]) != schemeID {
					newSchemes = append(newSchemes, s)
				}
			}
			schemes = newSchemes
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: `resource "atlassian_jira_screen_scheme" "test" {
					name        = "Original Scheme"
					description = "Original description"
					screens = {
						default = "10001"
					}
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "name", "Original Scheme"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "description", "Original description"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "screens.default", "10001"),
				),
			},
			// Update name, description, and screens
			{
				Config: `resource "atlassian_jira_screen_scheme" "test" {
					name        = "Updated Scheme"
					description = "Updated description"
					screens = {
						default = "10001"
						create  = "10002"
						view    = "10003"
						edit    = "10004"
					}
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "name", "Updated Scheme"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "screens.default", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "screens.create", "10002"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "screens.view", "10003"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "screens.edit", "10004"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenSchemeResource_delete — verifies delete removes resource
// ---------------------------------------------------------------------------

func TestAccScreenSchemeResource_delete(t *testing.T) {
	var mu sync.Mutex
	schemes := []map[string]interface{}{}
	var nextID atomic.Int64
	nextID.Store(30000)
	var deleteCalled atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screenscheme":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			id := nextID.Add(1)
			screens := map[string]int64{}
			if s, ok := body["screens"].(map[string]interface{}); ok {
				for k, v := range s {
					if val, ok := v.(float64); ok {
						screens[k] = int64(val)
					}
				}
			}
			scheme := screenSchemeJSON(id, body["name"].(string), "", screens)
			mu.Lock()
			schemes = append(schemes, scheme)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": id}) //nolint:errcheck

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screenscheme":
			mu.Lock()
			all := make([]map[string]interface{}, len(schemes))
			copy(all, schemes)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(paginatedScreenSchemeResponse(all)) //nolint:errcheck

		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screenscheme/"):
			deleteCalled.Add(1)
			schemeID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/screenscheme/")
			mu.Lock()
			newSchemes := []map[string]interface{}{}
			for _, s := range schemes {
				if fmt.Sprintf("%v", s["id"]) != schemeID {
					newSchemes = append(newSchemes, s)
				}
			}
			schemes = newSchemes
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if deleteCalled.Load() == 0 {
				return fmt.Errorf("expected delete to be called")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `resource "atlassian_jira_screen_scheme" "test" {
					name = "Delete Me Scheme"
					screens = {
						default = "10001"
					}
				}`,
				Check: resource.TestCheckResourceAttrSet("atlassian_jira_screen_scheme.test", "id"),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenSchemeResource_notFound — Read removes from state when 404
// ---------------------------------------------------------------------------

func TestAccScreenSchemeResource_notFound(t *testing.T) {
	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screenscheme":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": 40001}) //nolint:errcheck

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screenscheme":
			count := readCount.Add(1)
			if count <= 1 {
				// First read: return the scheme
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(paginatedScreenSchemeResponse([]map[string]interface{}{ //nolint:errcheck
					screenSchemeJSON(40001, "Gone Scheme", "", map[string]int64{"default": 10001}),
				}))
			} else {
				// Subsequent reads: scheme is gone
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(paginatedScreenSchemeResponse([]map[string]interface{}{})) //nolint:errcheck
			}

		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screenscheme/"):
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create and verify
			{
				Config: `resource "atlassian_jira_screen_scheme" "test" {
					name = "Gone Scheme"
					screens = {
						default = "10001"
					}
				}`,
				Check: resource.TestCheckResourceAttrSet("atlassian_jira_screen_scheme.test", "id"),
			},
			// Step 2: scheme disappears externally — plan should detect drift
			{
				Config: `resource "atlassian_jira_screen_scheme" "test" {
					name = "Gone Scheme"
					screens = {
						default = "10001"
					}
				}`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenSchemeDataSource_basic — data source lookup by name
// ---------------------------------------------------------------------------

func TestAccScreenSchemeDataSource_basic(t *testing.T) {
	existingScheme := screenSchemeJSON(50001, "DS Test Scheme", "DS description", map[string]int64{
		"default": 10001,
		"create":  10002,
	})

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screenscheme":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": 50001}) //nolint:errcheck

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screenscheme":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(paginatedScreenSchemeResponse([]map[string]interface{}{existingScheme})) //nolint:errcheck

		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screenscheme/"):
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "atlassian_jira_screen_scheme" "s" {
  name        = "DS Test Scheme"
  description = "DS description"
  screens = {
    default = "10001"
    create  = "10002"
  }
}
data "atlassian_jira_screen_scheme" "d" {
  name = atlassian_jira_screen_scheme.s.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_screen_scheme.d", "name", "DS Test Scheme"),
					resource.TestCheckResourceAttr("data.atlassian_jira_screen_scheme.d", "description", "DS description"),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_screen_scheme.d", "id"),
					resource.TestCheckResourceAttr("data.atlassian_jira_screen_scheme.d", "screens.default", "10001"),
					resource.TestCheckResourceAttr("data.atlassian_jira_screen_scheme.d", "screens.create", "10002"),
				),
			},
		},
	})
}
