package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

// ---------------------------------------------------------------------------
// Mock state helpers
// ---------------------------------------------------------------------------

type issueTypeScreenSchemeMockState struct {
	mu          sync.Mutex
	id          string
	name        string
	description string
	mappings    map[string]string // issueTypeId -> screenSchemeId
}

func (s *issueTypeScreenSchemeMockState) set(id, name, description string, mappings map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
	s.name = name
	s.description = description
	m := make(map[string]string, len(mappings))
	for k, v := range mappings {
		m[k] = v
	}
	s.mappings = m
}

func (s *issueTypeScreenSchemeMockState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = ""
	s.name = ""
	s.description = ""
	s.mappings = nil
}

func (s *issueTypeScreenSchemeMockState) schemeAPIResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]interface{}{
		"id":          s.id,
		"name":        s.name,
		"description": s.description,
	}
}

func (s *issueTypeScreenSchemeMockState) mappingItems() []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []map[string]interface{}
	for issueTypeID, screenSchemeID := range s.mappings {
		items = append(items, map[string]interface{}{
			"issueTypeScreenSchemeId": s.id,
			"issueTypeId":             issueTypeID,
			"screenSchemeId":          screenSchemeID,
		})
	}
	return items
}

// ---------------------------------------------------------------------------
// Mock server
// ---------------------------------------------------------------------------

func newIssueTypeScreenSchemeMockServer(state *issueTypeScreenSchemeMockState) *httptest.Server {
	const fixedID = "10002"

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Create
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/issuetypescreenscheme":
			var body struct {
				Name              string `json:"name"`
				Description       string `json:"description"`
				IssueTypeMappings []struct {
					IssueTypeID    string `json:"issueTypeId"`
					ScreenSchemeID string `json:"screenSchemeId"`
				} `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mappings := make(map[string]string)
			for _, m := range body.IssueTypeMappings {
				mappings[m.IssueTypeID] = m.ScreenSchemeID
			}
			state.set(fixedID, body.Name, body.Description, mappings)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"id": fixedID,
			})

		// List (for Read)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescreenscheme":
			state.mu.Lock()
			id := state.id
			state.mu.Unlock()
			if id == "" {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			json.NewEncoder(w).Encode(pageResponse([]interface{}{state.schemeAPIResponse()}))

		// Get mappings
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/mapping":
			json.NewEncoder(w).Encode(pageResponse(toInterfaceSlice(state.mappingItems())))

		// Update name/description
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/"+fixedID:
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.name = body.Name
			state.description = body.Description
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Add/update mappings
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/"+fixedID+"/mapping":
			var body struct {
				IssueTypeMappings []struct {
					IssueTypeID    string `json:"issueTypeId"`
					ScreenSchemeID string `json:"screenSchemeId"`
				} `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			for _, m := range body.IssueTypeMappings {
				state.mappings[m.IssueTypeID] = m.ScreenSchemeID
			}
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Remove mappings
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/"+fixedID+"/mapping/remove":
			var body struct {
				IssueTypeIDs []string `json:"issueTypeIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			for _, id := range body.IssueTypeIDs {
				delete(state.mappings, id)
			}
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Delete
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/"+fixedID:
			state.mu.Lock()
			id := state.id
			state.mu.Unlock()
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			state.clear()
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func toInterfaceSlice(items []map[string]interface{}) []interface{} {
	result := make([]interface{}, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

// ---------------------------------------------------------------------------
// Test 1: basic create/read lifecycle
// ---------------------------------------------------------------------------

func TestAccIssueTypeScreenSchemeResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeScreenSchemeMockState{}

	mockServer := newIssueTypeScreenSchemeMockServer(state)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name        = %q
  description = "A test issue type screen scheme"
  mappings = {
    "default" = "10001"
  }
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "description", "A test issue type screen scheme"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "id", "10002"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "mappings.default", "10001"),
				),
			},
			// Import
			{
				ResourceName:      "atlassian_jira_issue_type_screen_scheme.test",
				ImportState:       true,
				ImportStateId:     "10002",
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Test 2: update name/description, add/remove mappings
// ---------------------------------------------------------------------------

func TestAccIssueTypeScreenSchemeResource_update(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s-updated", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeScreenSchemeMockState{}

	mockServer := newIssueTypeScreenSchemeMockServer(state)
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
			// Initial create
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name        = %q
  description = "Original description"
  mappings = {
    "default" = "10001"
    "10003"   = "10004"
  }
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "mappings.default", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "mappings.10003", "10004"),
				),
			},
			// Update: rename, change description, remove 10003 mapping, add 10005 mapping
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name        = %q
  description = "Updated description"
  mappings = {
    "default" = "10001"
    "10005"   = "10006"
  }
}`, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "mappings.default", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "mappings.10005", "10006"),
					resource.TestCheckNoResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "mappings.10003"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Test 3: delete
// ---------------------------------------------------------------------------

func TestAccIssueTypeScreenSchemeResource_delete(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeScreenSchemeMockState{}

	mockServer := newIssueTypeScreenSchemeMockServer(state)
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.id != "" {
				return fmt.Errorf("expected issue type screen scheme to be deleted, still exists with id %q", state.id)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name     = %q
  mappings = {
    "default" = "10001"
  }
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "id", "10002"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Test 4: Read removes from state when 404 (not found)
// ---------------------------------------------------------------------------

func TestAccIssueTypeScreenSchemeResource_notFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeScreenSchemeMockState{}
	const fixedID = "10002"

	var listCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/issuetypescreenscheme":
			var body struct {
				Name              string `json:"name"`
				Description       string `json:"description"`
				IssueTypeMappings []struct {
					IssueTypeID    string `json:"issueTypeId"`
					ScreenSchemeID string `json:"screenSchemeId"`
				} `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mappings := make(map[string]string)
			for _, m := range body.IssueTypeMappings {
				mappings[m.IssueTypeID] = m.ScreenSchemeID
			}
			state.set(fixedID, body.Name, body.Description, mappings)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"id": fixedID,
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescreenscheme":
			listCount.Add(1)
			if listCount.Load() <= 1 {
				// First read: scheme exists
				json.NewEncoder(w).Encode(pageResponse([]interface{}{state.schemeAPIResponse()}))
			} else {
				// Subsequent reads: scheme gone
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
			}

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/mapping":
			json.NewEncoder(w).Encode(pageResponse(toInterfaceSlice(state.mappingItems())))

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/"+fixedID:
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
			// Initial create
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name     = %q
  mappings = { "default" = "10001" }
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "id", fixedID),
				),
			},
			// Second plan: scheme gone out-of-band → expect non-empty plan (re-create)
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name     = %q
  mappings = { "default" = "10001" }
}`, name),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Test 5: data source lookup
// ---------------------------------------------------------------------------

func TestAccIssueTypeScreenSchemeDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeScreenSchemeMockState{}

	// Use a mock server that also serves the list endpoint (same as the resource mock)
	mockServer := newIssueTypeScreenSchemeMockServer(state)
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
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name        = %q
  description = "Data source test"
  mappings = {
    "default" = "10001"
  }
}

data "atlassian_jira_issue_type_screen_scheme" "test" {
  name = atlassian_jira_issue_type_screen_scheme.test.name
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_screen_scheme.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_screen_scheme.test", "id", "10002"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_screen_scheme.test", "description", "Data source test"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Test 6: project association create/read
// ---------------------------------------------------------------------------

func TestAccProjectIssueTypeScreenSchemeResource_basic(t *testing.T) {
	projectID := "10500"
	schemeID := "10002"

	var mu sync.Mutex
	currentSchemeID := schemeID

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Assign scheme to project
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/project":
			var body struct {
				IssueTypeScreenSchemeID string `json:"issueTypeScreenSchemeId"`
				ProjectID               string `json:"projectId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.ProjectID != projectID {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			mu.Lock()
			currentSchemeID = body.IssueTypeScreenSchemeID
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Read: list issue type screen schemes for project
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescreenscheme/project":
			qProjectID := r.URL.Query().Get("projectId")
			if qProjectID != projectID {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			mu.Lock()
			sid := currentSchemeID
			mu.Unlock()
			json.NewEncoder(w).Encode(pageResponse([]interface{}{
				map[string]interface{}{
					"issueTypeScreenScheme": map[string]interface{}{
						"id":          sid,
						"name":        "Test ITS Scheme",
						"description": "",
					},
					"projectIds": []string{projectID},
				},
			}))

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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_issue_type_screen_scheme" "test" {
  project_id                  = %q
  issue_type_screen_scheme_id = %q
}`, projectID, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_issue_type_screen_scheme.test", "project_id", projectID),
					resource.TestCheckResourceAttr("atlassian_jira_project_issue_type_screen_scheme.test", "issue_type_screen_scheme_id", schemeID),
				),
			},
			// Import (by projectId only; resource resolves scheme via API)
			{
				ResourceName:                         "atlassian_jira_project_issue_type_screen_scheme.test",
				ImportState:                          true,
				ImportStateId:                        projectID,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_id",
			},
		},
	})
}
