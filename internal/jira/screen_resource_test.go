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

func screenJSON(id int64, name, description string) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
	}
}

func tabJSON(id int64, name string) map[string]interface{} {
	return map[string]interface{}{"id": id, "name": name}
}

func fieldJSON(id, name string) map[string]interface{} {
	return map[string]interface{}{"id": id, "name": name}
}

// ---------------------------------------------------------------------------
// TestAccScreenResource_basic — create → verify → update → import → destroy
// ---------------------------------------------------------------------------

func TestAccScreenResource_basic(t *testing.T) {
	var mu sync.Mutex
	screens := []map[string]interface{}{} // [{id, name, description}]
	var nextID atomic.Int64
	nextID.Store(1001)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// ---- Create screen
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screens":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			id := nextID.Add(1)
			s := screenJSON(id, body["name"], body["description"])
			mu.Lock()
			screens = append(screens, s)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(s)

		// ---- Update screen
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screens/"):
			screenID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/screens/")
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			var updated map[string]interface{}
			for i, s := range screens {
				if fmt.Sprintf("%v", s["id"]) == screenID {
					screens[i]["name"] = body["name"]
					screens[i]["description"] = body["description"]
					updated = screens[i]
					break
				}
			}
			mu.Unlock()
			if updated == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(updated)

		// ---- Delete screen
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screens/"):
			screenID := strings.TrimPrefix(r.URL.Path, "/rest/api/3/screens/")
			mu.Lock()
			newScreens := []map[string]interface{}{}
			for _, s := range screens {
				if fmt.Sprintf("%v", s["id"]) != screenID {
					newScreens = append(newScreens, s)
				}
			}
			screens = newScreens
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// ---- List screens (paginated)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screens":
			mu.Lock()
			all := make([]map[string]interface{}, len(screens))
			copy(all, screens)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":     all,
				"startAt":    0,
				"maxResults": 100,
				"total":      len(all),
				"isLast":     true,
			})

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
			return nil
		},
		Steps: []resource.TestStep{
			// Create and verify
			{
				Config: `resource "atlassian_jira_screen" "test" {
					name        = "Test Screen"
					description = "Initial description"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_screen.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "name", "Test Screen"),
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "description", "Initial description"),
				),
			},
			// Update name and description
			{
				Config: `resource "atlassian_jira_screen" "test" {
					name        = "Updated Screen"
					description = "Updated description"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "name", "Updated Screen"),
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "description", "Updated description"),
				),
			},
			// Import
			{
				ResourceName:      "atlassian_jira_screen.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenResource_Read_NotFound — 404 → RemoveResource
// ---------------------------------------------------------------------------

func TestAccScreenResource_Read_NotFound(t *testing.T) {
	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screens":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(screenJSON(5001, "Gone Screen", ""))

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screens":
			readCount.Add(1)
			if readCount.Load() <= 1 {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"values":  []map[string]interface{}{screenJSON(5001, "Gone Screen", "")},
					"isLast":  true,
					"startAt": 0, "maxResults": 100, "total": 1,
				})
			} else {
				// Simulate resource gone
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"values":  []map[string]interface{}{},
					"isLast":  true,
					"startAt": 0, "maxResults": 100, "total": 0,
				})
			}

		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screens/"):
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
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `resource "atlassian_jira_screen" "test" {
					name = "Gone Screen"
				}`,
			},
			{
				Config: `resource "atlassian_jira_screen" "test" {
					name = "Gone Screen"
				}`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenTabResource_basic — create screen → create tab → verify → import
// ---------------------------------------------------------------------------

func TestAccScreenTabResource_basic(t *testing.T) {
	var mu sync.Mutex
	screens := []map[string]interface{}{}
	tabs := map[string][]map[string]interface{}{} // screenID → []tab
	var nextScreenID, nextTabID atomic.Int64
	nextScreenID.Store(2000)
	nextTabID.Store(3000)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// ---- Create screen
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screens":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			id := nextScreenID.Add(1)
			s := screenJSON(id, body["name"], body["description"])
			mu.Lock()
			screens = append(screens, s)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(s)

		// ---- List screens
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screens":
			mu.Lock()
			all := make([]map[string]interface{}, len(screens))
			copy(all, screens)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": all, "isLast": true, "startAt": 0, "maxResults": 100, "total": len(all),
			})

		// ---- Delete screen
		case r.Method == "DELETE" && isScreenPath(r.URL.Path) && !isTabPath(r.URL.Path):
			w.WriteHeader(http.StatusNoContent)

		// ---- Create tab
		case r.Method == "POST" && isTabsListPath(r.URL.Path):
			screenID := extractScreenID(r.URL.Path)
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			id := nextTabID.Add(1)
			tab := tabJSON(id, body["name"])
			mu.Lock()
			tabs[screenID] = append(tabs[screenID], tab)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(tab)

		// ---- Update tab
		case r.Method == "PUT" && isTabPath(r.URL.Path):
			screenID, tabID := extractScreenAndTabID(r.URL.Path)
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			mu.Lock()
			for i, tab := range tabs[screenID] {
				if fmt.Sprintf("%v", tab["id"]) == tabID {
					tabs[screenID][i]["name"] = body["name"]
					break
				}
			}
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": tabID, "name": body["name"]})

		// ---- List tabs
		case r.Method == "GET" && isTabsListPath(r.URL.Path):
			screenID := extractScreenID(r.URL.Path)
			mu.Lock()
			all := make([]map[string]interface{}, len(tabs[screenID]))
			copy(all, tabs[screenID])
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(all)

		// ---- Delete tab
		case r.Method == "DELETE" && isTabPath(r.URL.Path):
			screenID, tabID := extractScreenAndTabID(r.URL.Path)
			mu.Lock()
			newTabs := []map[string]interface{}{}
			for _, tab := range tabs[screenID] {
				if fmt.Sprintf("%v", tab["id"]) != tabID {
					newTabs = append(newTabs, tab)
				}
			}
			tabs[screenID] = newTabs
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
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "atlassian_jira_screen" "s" {
  name = "Tab Test Screen"
}
resource "atlassian_jira_screen_tab" "t" {
  screen_id = atlassian_jira_screen.s.id
  name      = "My Tab"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_tab.t", "id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_tab.t", "screen_id"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_tab.t", "name", "My Tab"),
				),
			},
			// Import tab
			{
				ResourceName:      "atlassian_jira_screen_tab.t",
				ImportState:       true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					screenID := s.RootModule().Resources["atlassian_jira_screen.s"].Primary.ID
					tabID := s.RootModule().Resources["atlassian_jira_screen_tab.t"].Primary.ID
					return screenID + "/" + tabID, nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenTabFieldResource_basic — screen → tab → add field → import
// ---------------------------------------------------------------------------

func TestAccScreenTabFieldResource_basic(t *testing.T) {
	var mu sync.Mutex
	screens := []map[string]interface{}{}
	tabs := map[string][]map[string]interface{}{}
	fields := map[string][]map[string]interface{}{} // "screenID/tabID" → []field
	var nextScreenID, nextTabID atomic.Int64
	nextScreenID.Store(4000)
	nextTabID.Store(5000)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// ---- Create screen
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screens":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			id := nextScreenID.Add(1)
			s := screenJSON(id, body["name"], body["description"])
			mu.Lock()
			screens = append(screens, s)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(s)

		// ---- List screens
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screens":
			mu.Lock()
			all := make([]map[string]interface{}, len(screens))
			copy(all, screens)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": all, "isLast": true, "startAt": 0, "maxResults": 100, "total": len(all),
			})

		// ---- Delete screen
		case r.Method == "DELETE" && isScreenPath(r.URL.Path) && !isTabPath(r.URL.Path):
			w.WriteHeader(http.StatusNoContent)

		// ---- Create tab
		case r.Method == "POST" && isTabsListPath(r.URL.Path):
			screenID := extractScreenID(r.URL.Path)
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			id := nextTabID.Add(1)
			tab := tabJSON(id, body["name"])
			mu.Lock()
			tabs[screenID] = append(tabs[screenID], tab)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(tab)

		// ---- List tabs
		case r.Method == "GET" && isTabsListPath(r.URL.Path):
			screenID := extractScreenID(r.URL.Path)
			mu.Lock()
			all := make([]map[string]interface{}, len(tabs[screenID]))
			copy(all, tabs[screenID])
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(all)

		// ---- Delete tab
		case r.Method == "DELETE" && isTabPath(r.URL.Path) && !isFieldPath(r.URL.Path):
			w.WriteHeader(http.StatusNoContent)

		// ---- Add field to tab
		case r.Method == "POST" && isFieldsListPath(r.URL.Path):
			screenID, tabID := extractScreenAndTabIDFromFields(r.URL.Path)
			key := screenID + "/" + tabID
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			fid := body["fieldId"]
			f := fieldJSON(fid, fid)
			mu.Lock()
			fields[key] = append(fields[key], f)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(f)

		// ---- List fields on tab
		case r.Method == "GET" && isFieldsListPath(r.URL.Path):
			screenID, tabID := extractScreenAndTabIDFromFields(r.URL.Path)
			key := screenID + "/" + tabID
			mu.Lock()
			all := make([]map[string]interface{}, len(fields[key]))
			copy(all, fields[key])
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(all)

		// ---- Delete field from tab
		case r.Method == "DELETE" && isFieldPath(r.URL.Path):
			screenID, tabID, fieldID := extractScreenTabFieldID(r.URL.Path)
			key := screenID + "/" + tabID
			mu.Lock()
			newFields := []map[string]interface{}{}
			for _, f := range fields[key] {
				if fmt.Sprintf("%v", f["id"]) != fieldID {
					newFields = append(newFields, f)
				}
			}
			fields[key] = newFields
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
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "atlassian_jira_screen" "s" {
  name = "Field Test Screen"
}
resource "atlassian_jira_screen_tab" "t" {
  screen_id = atlassian_jira_screen.s.id
  name      = "Field Tab"
}
resource "atlassian_jira_screen_tab_field" "f" {
  screen_id = atlassian_jira_screen.s.id
  tab_id    = atlassian_jira_screen_tab.t.id
  field_id  = "summary"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen_tab_field.f", "field_id", "summary"),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_tab_field.f", "screen_id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_tab_field.f", "tab_id"),
				),
			},
			// Import field — no id attribute, verify individual attributes manually
			{
				ResourceName: "atlassian_jira_screen_tab_field.f",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					screenID := s.RootModule().Resources["atlassian_jira_screen.s"].Primary.ID
					tabID := s.RootModule().Resources["atlassian_jira_screen_tab.t"].Primary.ID
					fieldID := s.RootModule().Resources["atlassian_jira_screen_tab_field.f"].Primary.Attributes["field_id"]
					return screenID + "/" + tabID + "/" + fieldID, nil
				},
				ImportStateVerify: false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["field_id"] != "summary" {
						return fmt.Errorf("expected field_id=summary, got %q", attrs["field_id"])
					}
					if attrs["screen_id"] == "" {
						return fmt.Errorf("expected screen_id to be set")
					}
					if attrs["tab_id"] == "" {
						return fmt.Errorf("expected tab_id to be set")
					}
					return nil
				},
			},
		},
	})
}

// ---------------------------------------------------------------------------
// TestAccScreenDataSource_basic
// ---------------------------------------------------------------------------

func TestAccScreenDataSource_basic(t *testing.T) {
	screens := []map[string]interface{}{
		screenJSON(9001, "DS Test Screen", "DS description"),
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/screens":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(screens[0])

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/screens":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": screens, "isLast": true, "startAt": 0, "maxResults": 100, "total": len(screens),
			})

		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/screens/"):
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
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "atlassian_jira_screen" "s" {
  name        = "DS Test Screen"
  description = "DS description"
}
data "atlassian_jira_screen" "d" {
  name = atlassian_jira_screen.s.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_screen.d", "name", "DS Test Screen"),
					resource.TestCheckResourceAttr("data.atlassian_jira_screen.d", "description", "DS description"),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_screen.d", "id"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// URL path helper functions
// ---------------------------------------------------------------------------

// isScreenPath matches /rest/api/3/screens/{screenId}
func isScreenPath(p string) bool {
	parts := strings.Split(strings.TrimPrefix(p, "/rest/api/3/screens/"), "/")
	return strings.HasPrefix(p, "/rest/api/3/screens/") && len(parts) >= 1 && parts[0] != ""
}

// isTabsListPath matches /rest/api/3/screens/{screenId}/tabs (no tabId segment)
func isTabsListPath(p string) bool {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return len(parts) == 2 && parts[1] == "tabs"
}

// isTabPath matches /rest/api/3/screens/{screenId}/tabs/{tabId}
func isTabPath(p string) bool {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return len(parts) == 3 && parts[1] == "tabs" && parts[2] != ""
}

// isFieldsListPath matches /rest/api/3/screens/{screenId}/tabs/{tabId}/fields
func isFieldsListPath(p string) bool {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return len(parts) == 4 && parts[1] == "tabs" && parts[3] == "fields"
}

// isFieldPath matches /rest/api/3/screens/{screenId}/tabs/{tabId}/fields/{fieldId}
func isFieldPath(p string) bool {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return len(parts) == 5 && parts[1] == "tabs" && parts[3] == "fields" && parts[4] != ""
}

func extractScreenID(p string) string {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	return strings.Split(suffix, "/")[0]
}

func extractScreenAndTabID(p string) (string, string) {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return parts[0], parts[2]
}

func extractScreenAndTabIDFromFields(p string) (string, string) {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return parts[0], parts[2]
}

func extractScreenTabFieldID(p string) (string, string, string) {
	suffix := strings.TrimPrefix(p, "/rest/api/3/screens/")
	parts := strings.Split(suffix, "/")
	return parts[0], parts[2], parts[4]
}
