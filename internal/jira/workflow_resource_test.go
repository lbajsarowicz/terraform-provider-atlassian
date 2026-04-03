package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

const workflowFixedEntityID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

type workflowMockState struct {
	mu          sync.Mutex
	entityID    string
	name        string
	description string
	statuses    []string
}

func (s *workflowMockState) set(entityID, name, description string, statuses []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entityID = entityID
	s.name = name
	s.description = description
	s.statuses = statuses
}

func (s *workflowMockState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entityID = ""
	s.name = ""
	s.description = ""
	s.statuses = nil
}

func (s *workflowMockState) get() (string, string, string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	statusesCopy := make([]string, len(s.statuses))
	copy(statusesCopy, s.statuses)
	return s.entityID, s.name, s.description, statusesCopy
}

func (s *workflowMockState) apiItem() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	statusList := make([]map[string]interface{}, len(s.statuses))
	for i, sid := range s.statuses {
		statusList[i] = map[string]interface{}{
			"id":              sid,
			"statusReference": sid,
			"name":            "Status " + sid,
			"statusCategory":  "TODO",
		}
	}
	return map[string]interface{}{
		"id": map[string]interface{}{
			"entityId": s.entityID,
			"name":     s.name,
		},
		"description": s.description,
		"statuses":    statusList,
		"transitions": []interface{}{},
	}
}

func newWorkflowMockServer(state *workflowMockState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/workflow":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Statuses    []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			statusIDs := make([]string, len(body.Statuses))
			for i, s := range body.Statuses {
				statusIDs[i] = s.ID
			}
			state.set(workflowFixedEntityID, body.Name, body.Description, statusIDs)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"entityId": workflowFixedEntityID,
				"name":     body.Name,
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflow/search":
			entityID, _, _, _ := state.get()
			if entityID == "" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"values": []interface{}{},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []interface{}{state.apiItem()},
			})

		case r.Method == "DELETE":
			entityID, _, _, _ := state.get()
			expectedPath := "/rest/api/3/workflow/" + entityID
			if r.URL.Path != expectedPath || entityID == "" {
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

func TestAccWorkflowResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &workflowMockState{}

	mockServer := newWorkflowMockServer(state)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow" "test" {
  name        = %q
  description = "A test workflow"
  statuses    = ["status-uuid-1", "status-uuid-2"]
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "description", "A test workflow"),
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "id", workflowFixedEntityID),
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "statuses.#", "2"),
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "statuses.0", "status-uuid-1"),
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "statuses.1", "status-uuid-2"),
				),
			},
		},
	})
}

func TestAccWorkflowResource_Read_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &workflowMockState{}

	var readCount int
	var mu sync.Mutex

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/workflow":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Statuses    []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			statusIDs := make([]string, len(body.Statuses))
			for i, s := range body.Statuses {
				statusIDs[i] = s.ID
			}
			state.set(workflowFixedEntityID, body.Name, body.Description, statusIDs)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"entityId": workflowFixedEntityID,
				"name":     body.Name,
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/workflow/search":
			mu.Lock()
			readCount++
			current := readCount
			mu.Unlock()

			if current <= 1 {
				// First read: return the workflow
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"values": []interface{}{state.apiItem()},
				})
			} else {
				// Subsequent reads: return empty (workflow deleted out-of-band)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"values": []interface{}{},
				})
			}

		case r.Method == "DELETE":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow" "test" {
  name     = %q
  statuses = ["status-uuid-1"]
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "name", name),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow" "test" {
  name     = %q
  statuses = ["status-uuid-1"]
}`, name),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccWorkflowResource_Import(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &workflowMockState{}

	mockServer := newWorkflowMockServer(state)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_workflow" "test" {
  name        = %q
  description = "Import test workflow"
  statuses    = ["status-uuid-1"]
}`, name),
			},
			{
				ResourceName:      "atlassian_jira_workflow.test",
				ImportState:       true,
				ImportStateId:     workflowFixedEntityID,
				ImportStateVerify: true,
				// description and statuses are re-read from API after import
			},
		},
	})
}

func TestAccWorkflowDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &workflowMockState{}

	mockServer := newWorkflowMockServer(state)
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
resource "atlassian_jira_workflow" "test" {
  name        = %q
  description = "Data source test workflow"
  statuses    = ["status-uuid-1", "status-uuid-2"]
}

data "atlassian_jira_workflow" "test" {
  name = atlassian_jira_workflow.test.name
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow.test", "id", workflowFixedEntityID),
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow.test", "description", "Data source test workflow"),
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow.test", "statuses.#", "2"),
				),
			},
		},
	})
}
