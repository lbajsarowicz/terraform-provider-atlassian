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

type workflowSchemeMockState struct {
	mu                sync.Mutex
	id                int64
	name              string
	description       string
	defaultWorkflow   string
	issueTypeMappings map[string]string
}

func (s *workflowSchemeMockState) set(id int64, name, description, defaultWorkflow string, mappings map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
	s.name = name
	s.description = description
	s.defaultWorkflow = defaultWorkflow
	s.issueTypeMappings = mappings
}

func (s *workflowSchemeMockState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = 0
	s.name = ""
	s.description = ""
	s.defaultWorkflow = ""
	s.issueTypeMappings = nil
}

func (s *workflowSchemeMockState) apiResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	mappings := map[string]string{}
	for k, v := range s.issueTypeMappings {
		mappings[k] = v
	}
	return map[string]interface{}{
		"id":                s.id,
		"name":              s.name,
		"description":       s.description,
		"defaultWorkflow":   s.defaultWorkflow,
		"issueTypeMappings": mappings,
		"draft":             false,
	}
}

// ---------------------------------------------------------------------------
// Mock server for workflow scheme CRUD
// ---------------------------------------------------------------------------

func newWorkflowSchemeMockServer(state *workflowSchemeMockState) *httptest.Server {
	const fixedID int64 = 10000

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Create
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/workflowscheme":
			var body struct {
				Name              string            `json:"name"`
				Description       string            `json:"description"`
				DefaultWorkflow   string            `json:"defaultWorkflow"`
				IssueTypeMappings map[string]string `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set(fixedID, body.Name, body.Description, body.DefaultWorkflow, body.IssueTypeMappings)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse())

		// Read
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			state.mu.Lock()
			id := state.id
			state.mu.Unlock()
			if id == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(state.apiResponse())

		// Update
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			var body struct {
				Name              string            `json:"name"`
				Description       string            `json:"description"`
				DefaultWorkflow   string            `json:"defaultWorkflow"`
				IssueTypeMappings map[string]string `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set(fixedID, body.Name, body.Description, body.DefaultWorkflow, body.IssueTypeMappings)
			json.NewEncoder(w).Encode(state.apiResponse())

		// Delete
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			state.mu.Lock()
			id := state.id
			state.mu.Unlock()
			if id == 0 {
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

// ---------------------------------------------------------------------------
// Workflow scheme resource tests
// ---------------------------------------------------------------------------

func TestAccWorkflowSchemeResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &workflowSchemeMockState{}

	mockServer := newWorkflowSchemeMockServer(state)
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	updatedName := fmt.Sprintf("tf-test-%s-updated", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			// Create and verify
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "A test workflow scheme"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "description", "A test workflow scheme"),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "id", "10000"),
				),
			},
			// Update name and description
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "Updated description"
}`, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "id", "10000"),
				),
			},
			// Import
			{
				ResourceName:      "atlassian_jira_workflow_scheme.test",
				ImportState:       true,
				ImportStateId:     "10000",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccWorkflowSchemeResource_withMappings(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &workflowSchemeMockState{}

	mockServer := newWorkflowSchemeMockServer(state)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow_scheme" "test" {
  name             = %q
  description      = "Scheme with mappings"
  default_workflow = "jira"
  issue_type_mappings = {
    "10001" = "Bug Workflow"
  }
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "default_workflow", "jira"),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "issue_type_mappings.10001", "Bug Workflow"),
				),
			},
		},
	})
}

func TestAccWorkflowSchemeResource_Read_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &workflowSchemeMockState{}

	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/workflowscheme":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set(10000, body.Name, body.Description, "", nil)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse())

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			readCount.Add(1)
			if readCount.Load() <= 1 {
				json.NewEncoder(w).Encode(state.apiResponse())
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow_scheme" "test" {
  name = %q
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", name),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow_scheme" "test" {
  name = %q
}`, name),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Project workflow scheme resource tests
// ---------------------------------------------------------------------------

func newProjectWorkflowSchemeMockServer(projectID, schemeID string) *httptest.Server {
	var mu sync.Mutex
	currentSchemeID := schemeID

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Assign scheme to project
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/workflowscheme/project":
			var body struct {
				WorkflowSchemeID string `json:"workflowSchemeId"`
				ProjectID        string `json:"projectId"`
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
			currentSchemeID = body.WorkflowSchemeID
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Read: list workflow schemes for project
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme/project":
			qProjectID := r.URL.Query().Get("projectId")
			if qProjectID != projectID {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			mu.Lock()
			sid := currentSchemeID
			mu.Unlock()
			// Parse scheme ID to int for the response shape
			var schemeIDInt int64
			fmt.Sscanf(sid, "%d", &schemeIDInt)
			json.NewEncoder(w).Encode(pageResponse([]interface{}{
				map[string]interface{}{
					"workflowScheme": map[string]interface{}{
						"id":          schemeIDInt,
						"name":        "Test Scheme",
						"description": "",
						"draft":       false,
					},
				},
			}))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccProjectWorkflowSchemeResource_basic(t *testing.T) {
	projectID := "10400"
	schemeID := "10000"

	mockServer := newProjectWorkflowSchemeMockServer(projectID, schemeID)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_workflow_scheme" "test" {
  project_id         = %q
  workflow_scheme_id = %q
}`, projectID, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_workflow_scheme.test", "project_id", projectID),
					resource.TestCheckResourceAttr("atlassian_jira_project_workflow_scheme.test", "workflow_scheme_id", schemeID),
				),
			},
			// Import
			{
				ResourceName:                         "atlassian_jira_project_workflow_scheme.test",
				ImportState:                          true,
				ImportStateId:                        projectID,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_id",
			},
		},
	})
}

func TestAccProjectWorkflowSchemeResource_notFound(t *testing.T) {
	projectID := "10401"
	schemeID := "10001"

	var mu sync.Mutex
	found := true

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/workflowscheme/project":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme/project":
			mu.Lock()
			f := found
			mu.Unlock()
			if !f {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			var schemeIDInt int64
			fmt.Sscanf(schemeID, "%d", &schemeIDInt)
			json.NewEncoder(w).Encode(pageResponse([]interface{}{
				map[string]interface{}{
					"workflowScheme": map[string]interface{}{
						"id":          schemeIDInt,
						"name":        "Test Scheme",
						"description": "",
						"draft":       false,
					},
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_workflow_scheme" "test" {
  project_id         = %q
  workflow_scheme_id = %q
}`, projectID, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_workflow_scheme.test", "project_id", projectID),
				),
			},
			{
				// Simulate out-of-band removal
				PreConfig: func() {
					mu.Lock()
					found = false
					mu.Unlock()
				},
				Config: fmt.Sprintf(`resource "atlassian_jira_project_workflow_scheme" "test" {
  project_id         = %q
  workflow_scheme_id = %q
}`, projectID, schemeID),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Workflow scheme data source tests
// ---------------------------------------------------------------------------

func newWorkflowSchemeListMockServer(state *workflowSchemeMockState) *httptest.Server {
	const fixedID int64 = 10000

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Create
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/workflowscheme":
			var body struct {
				Name              string            `json:"name"`
				Description       string            `json:"description"`
				DefaultWorkflow   string            `json:"defaultWorkflow"`
				IssueTypeMappings map[string]string `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set(fixedID, body.Name, body.Description, body.DefaultWorkflow, body.IssueTypeMappings)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse())

		// Read (single)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			state.mu.Lock()
			id := state.id
			state.mu.Unlock()
			if id == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(state.apiResponse())

		// List (for data source)
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme":
			state.mu.Lock()
			id := state.id
			state.mu.Unlock()
			if id == 0 {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			json.NewEncoder(w).Encode(pageResponse([]interface{}{state.apiResponse()}))

		// Update
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			var body struct {
				Name              string            `json:"name"`
				Description       string            `json:"description"`
				DefaultWorkflow   string            `json:"defaultWorkflow"`
				IssueTypeMappings map[string]string `json:"issueTypeMappings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set(fixedID, body.Name, body.Description, body.DefaultWorkflow, body.IssueTypeMappings)
			json.NewEncoder(w).Encode(state.apiResponse())

		// Delete
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/workflowscheme/10000":
			state.clear()
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccWorkflowSchemeDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &workflowSchemeMockState{}

	mockServer := newWorkflowSchemeListMockServer(state)
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
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "Data source test scheme"
}

data "atlassian_jira_workflow_scheme" "test" {
  name = atlassian_jira_workflow_scheme.test.name
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow_scheme.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow_scheme.test", "id", "10000"),
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow_scheme.test", "description", "Data source test scheme"),
				),
			},
		},
	})
}
