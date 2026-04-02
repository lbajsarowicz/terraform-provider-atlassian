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

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

// issueTypeSchemeState holds mock server state for an issue type scheme.
type issueTypeSchemeState struct {
	mu                 sync.Mutex
	ID                 string
	Name               string
	Description        string
	DefaultIssueTypeID string
	IssueTypeIDs       []string
}

func (s *issueTypeSchemeState) set(id, name, description, defaultIssueTypeID string, issueTypeIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ID = id
	s.Name = name
	s.Description = description
	s.DefaultIssueTypeID = defaultIssueTypeID
	ids := make([]string, len(issueTypeIDs))
	copy(ids, issueTypeIDs)
	s.IssueTypeIDs = ids
}

func (s *issueTypeSchemeState) schemeAPIResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]interface{}{
		"id":          s.ID,
		"name":        s.Name,
		"description": s.Description,
		"isDefault":   false,
	}
	if s.DefaultIssueTypeID != "" {
		m["defaultIssueTypeId"] = s.DefaultIssueTypeID
	}
	return m
}

func (s *issueTypeSchemeState) itemsResponse() []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []map[string]interface{}
	for _, id := range s.IssueTypeIDs {
		items = append(items, map[string]interface{}{
			"issueTypeSchemeId": s.ID,
			"issueTypeId":       id,
		})
	}
	return items
}

// pageResponse wraps values in a Jira-style paginated response.
func pageResponse(values interface{}) map[string]interface{} {
	return map[string]interface{}{
		"startAt":    0,
		"maxResults": 50,
		"total":      1,
		"isLast":     true,
		"values":     values,
	}
}

func newIssueTypeSchemeMockServer(state *issueTypeSchemeState, readCount *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Create scheme
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/issuetypescheme":
			var body struct {
				Name               string   `json:"name"`
				Description        string   `json:"description"`
				DefaultIssueTypeID string   `json:"defaultIssueTypeId"`
				IssueTypeIDs       []string `json:"issueTypeIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set("10100", body.Name, body.Description, body.DefaultIssueTypeID, body.IssueTypeIDs)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"issueTypeSchemeId": "10100"})

		// List all schemes (paginated)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme":
			var count int32
			if readCount != nil {
				count = readCount.Add(1)
			}
			s := state.schemeAPIResponse()
			if s["id"] == "" || (readCount != nil && count > 1) {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			json.NewEncoder(w).Encode(pageResponse([]interface{}{s}))

		// List scheme items (paginated)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme/mapping":
			items := state.itemsResponse()
			result := make([]interface{}, len(items))
			for i, item := range items {
				result[i] = item
			}
			json.NewEncoder(w).Encode(pageResponse(result))

		// Update scheme properties
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescheme/10100":
			var body struct {
				Name               string `json:"name"`
				Description        string `json:"description"`
				DefaultIssueTypeID string `json:"defaultIssueTypeId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.Name = body.Name
			state.Description = body.Description
			state.DefaultIssueTypeID = body.DefaultIssueTypeID
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Add issue types to scheme
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescheme/10100/issuetype":
			var body struct {
				IssueTypeIDs []string `json:"issueTypeIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.IssueTypeIDs = append(state.IssueTypeIDs, body.IssueTypeIDs...)
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Move/reorder issue types
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescheme/10100/issuetype/move":
			var body struct {
				IssueTypeIDs []string `json:"issueTypeIds"`
				Position     string   `json:"position"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.IssueTypeIDs = body.IssueTypeIDs
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Delete issue type from scheme
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/issuetypescheme/10100/issuetype/"):
			parts := strings.Split(r.URL.Path, "/")
			removedID := parts[len(parts)-1]
			state.mu.Lock()
			var updated []string
			for _, id := range state.IssueTypeIDs {
				if id != removedID {
					updated = append(updated, id)
				}
			}
			state.IssueTypeIDs = updated
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Delete scheme
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/issuetypescheme/10100":
			id := state.schemeAPIResponse()["id"]
			if id == "" || id == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			state.set("", "", "", "", nil)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccIssueTypeSchemeResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &issueTypeSchemeState{}

	mockServer := newIssueTypeSchemeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_scheme" "test" {
  name          = %q
  description   = "Test scheme"
  issue_type_ids = ["10001", "10002"]
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "description", "Test scheme"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "id", "10100"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "issue_type_ids.#", "2"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "issue_type_ids.0", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "issue_type_ids.1", "10002"),
				),
			},
		},
	})
}

func TestAccIssueTypeSchemeResource_update(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s-updated", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeSchemeState{}

	mockServer := newIssueTypeSchemeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_scheme" "test" {
  name          = %q
  description   = "Original description"
  issue_type_ids = ["10001"]
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "description", "Original description"),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_scheme" "test" {
  name          = %q
  description   = "Updated description"
  issue_type_ids = ["10001", "10002"]
}`, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "id", "10100"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "issue_type_ids.#", "2"),
				),
			},
		},
	})
}

func TestAccIssueTypeSchemeResource_import(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &issueTypeSchemeState{}

	mockServer := newIssueTypeSchemeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_scheme" "test" {
  name          = %q
  description   = "Import test"
  issue_type_ids = ["10001"]
}`, name),
			},
			{
				ResourceName:            "atlassian_jira_issue_type_scheme.test",
				ImportState:             true,
				ImportStateId:           "10100",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"description"},
			},
		},
	})
}

func TestAccIssueTypeSchemeResource_notFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &issueTypeSchemeState{}

	var readCount atomic.Int32
	mockServer := newIssueTypeSchemeMockServer(state, &readCount)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_scheme" "test" {
  name          = %q
  issue_type_ids = ["10001"]
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", name),
				),
			},
			{
				// On the second read (refresh in step 2), the scheme is gone.
				// The mock returns empty list once readCount > 1, simulating out-of-band deletion.
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type_scheme" "test" {
  name          = %q
  issue_type_ids = ["10001"]
}`, name),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccIssueTypeSchemeDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme":
			json.NewEncoder(w).Encode(pageResponse([]interface{}{
				map[string]interface{}{
					"id":                 "10200",
					"name":               name,
					"description":        "Data source test scheme",
					"defaultIssueTypeId": "10001",
					"isDefault":          false,
				},
			}))
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme/mapping":
			json.NewEncoder(w).Encode(pageResponse([]interface{}{
				map[string]interface{}{"issueTypeSchemeId": "10200", "issueTypeId": "10001"},
				map[string]interface{}{"issueTypeSchemeId": "10200", "issueTypeId": "10002"},
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
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "atlassian_jira_issue_type_scheme" "test" { name = %q }`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "id", "10200"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "description", "Data source test scheme"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "default_issue_type_id", "10001"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "issue_type_ids.#", "2"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "issue_type_ids.0", "10001"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "issue_type_ids.1", "10002"),
				),
			},
		},
	})
}
