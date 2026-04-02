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

type statusState struct {
	mu             sync.Mutex
	ID             string
	Name           string
	Description    string
	StatusCategory string
}

func (s *statusState) set(id, name, description, statusCategory string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ID = id
	s.Name = name
	s.Description = description
	s.StatusCategory = statusCategory
}

func (s *statusState) get() (string, string, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ID, s.Name, s.Description, s.StatusCategory
}

func (s *statusState) apiResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]interface{}{
		"id":          s.ID,
		"name":        s.Name,
		"description": s.Description,
		"statusCategory": map[string]interface{}{
			"key": s.StatusCategory,
		},
	}
}

func (s *statusState) searchResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ID == "" {
		return map[string]interface{}{
			"startAt":    0,
			"maxResults": 50,
			"total":      0,
			"isLast":     true,
			"values":     []interface{}{},
		}
	}
	return map[string]interface{}{
		"startAt":    0,
		"maxResults": 50,
		"total":      1,
		"isLast":     true,
		"values": []interface{}{
			map[string]interface{}{
				"id":          s.ID,
				"name":        s.Name,
				"description": s.Description,
				"statusCategory": map[string]interface{}{
					"key": s.StatusCategory,
				},
			},
		},
	}
}

func newStatusMockServer(state *statusState, readCount *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/statuses":
			var body struct {
				Statuses []struct {
					Name           string `json:"name"`
					Description    string `json:"description"`
					StatusCategory string `json:"statusCategory"`
				} `json:"statuses"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(body.Statuses) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s := body.Statuses[0]
			state.set("10001", s.Name, s.Description, s.StatusCategory)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]interface{}{state.apiResponse()})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/statuses/search":
			if readCount != nil {
				readCount.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(state.searchResponse())

		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/statuses":
			var body struct {
				Statuses []struct {
					ID             string `json:"id"`
					Name           string `json:"name"`
					Description    string `json:"description"`
					StatusCategory string `json:"statusCategory"`
				} `json:"statuses"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(body.Statuses) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s := body.Statuses[0]
			state.set(s.ID, s.Name, s.Description, s.StatusCategory)
			w.WriteHeader(http.StatusOK)

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/statuses":
			idParam := r.URL.Query().Get("id")
			id, _, _, _ := state.get()
			if id == "" || id != idParam {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			state.set("", "", "", "")
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccStatusResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &statusState{}

	mockServer := newStatusMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "A test status"
  status_category = "TODO"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "description", "A test status"),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "status_category", "TODO"),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "id", "10001"),
				),
			},
		},
	})
}

func TestAccStatusResource_update(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s-updated", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &statusState{}

	mockServer := newStatusMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "Original description"
  status_category = "IN_PROGRESS"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "description", "Original description"),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "status_category", "IN_PROGRESS"),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "Updated description"
  status_category = "IN_PROGRESS"
}`, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "id", "10001"),
				),
			},
		},
	})
}

func TestAccStatusResource_Read_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &statusState{}

	var readCount atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/statuses":
			var body struct {
				Statuses []struct {
					Name           string `json:"name"`
					Description    string `json:"description"`
					StatusCategory string `json:"statusCategory"`
				} `json:"statuses"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(body.Statuses) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s := body.Statuses[0]
			state.set("10001", s.Name, s.Description, s.StatusCategory)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]interface{}{state.apiResponse()})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/statuses/search":
			readCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if readCount.Load() <= 1 {
				json.NewEncoder(w).Encode(state.searchResponse())
			} else {
				// Return empty — simulates out-of-band deletion
				json.NewEncoder(w).Encode(map[string]interface{}{
					"startAt":    0,
					"maxResults": 50,
					"total":      0,
					"isLast":     true,
					"values":     []interface{}{},
				})
			}

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/statuses":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_status" "test" {
  name            = %q
  status_category = "DONE"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", name),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_status" "test" {
  name            = %q
  status_category = "DONE"
}`, name),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccStatusResource_Import(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &statusState{}

	mockServer := newStatusMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "Import test"
  status_category = "TODO"
}`, name),
			},
			{
				ResourceName:      "atlassian_jira_status.test",
				ImportState:       true,
				ImportStateId:     "10001",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccStatusDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &statusState{}

	mockServer := newStatusMockServer(state, nil)
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
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "Data source test"
  status_category = "IN_PROGRESS"
}

data "atlassian_jira_status" "test" {
  name = atlassian_jira_status.test.name
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "description", "Data source test"),
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "status_category", "IN_PROGRESS"),
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "id", "10001"),
				),
			},
		},
	})
}
