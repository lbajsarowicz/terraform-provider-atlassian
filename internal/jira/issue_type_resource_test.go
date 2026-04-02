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

type issueTypeState struct {
	mu          sync.Mutex
	ID          string
	Name        string
	Description string
	Type        string
	AvatarID    int64
}

func (s *issueTypeState) set(id, name, description, typ string, avatarID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ID = id
	s.Name = name
	s.Description = description
	s.Type = typ
	s.AvatarID = avatarID
}

func (s *issueTypeState) get() (string, string, string, string, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ID, s.Name, s.Description, s.Type, s.AvatarID
}

func (s *issueTypeState) apiResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]interface{}{
		"id":             s.ID,
		"name":           s.Name,
		"description":    s.Description,
		"subtask":        s.Type == "subtask",
		"hierarchyLevel": hierarchyLevelFor(s.Type),
		"avatarId":       s.AvatarID,
	}
}

func hierarchyLevelFor(typ string) int64 {
	if typ == "subtask" {
		return -1
	}
	return 0
}

func newIssueTypeMockServer(state *issueTypeState, readCount *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/issuetype":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Type        string `json:"type"`
				AvatarID    int64  `json:"avatarId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.Type != "standard" && body.Type != "subtask" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid type"})
				return
			}
			state.set("10001", body.Name, body.Description, body.Type, body.AvatarID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse())

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetype/10001":
			if readCount != nil {
				readCount.Add(1)
			}
			id, _, _, _, _ := state.get()
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(state.apiResponse())

		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetype/10001":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Type        string `json:"type"`
				AvatarID    int64  `json:"avatarId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			id, _, _, typ, _ := state.get()
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			// type cannot change on update in real API, but we accept whatever is sent
			_ = typ
			state.set("10001", body.Name, body.Description, body.Type, body.AvatarID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(state.apiResponse())

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/issuetype/10001":
			id, _, _, _, _ := state.get()
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			state.set("", "", "", "", 0)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccIssueTypeResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeState{}

	mockServer := newIssueTypeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  description = "A test issue type"
  type = "standard"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "A test issue type"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "type", "standard"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "id", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "hierarchy_level", "0"),
				),
			},
		},
	})
}

func TestAccIssueTypeResource_subtask(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeState{}

	mockServer := newIssueTypeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "subtask"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "type", "subtask"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "hierarchy_level", "-1"),
				),
			},
		},
	})
}

func TestAccIssueTypeResource_update(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s-updated", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeState{}

	mockServer := newIssueTypeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  description = "Original description"
  type = "standard"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "Original description"),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  description = "Updated description"
  type = "standard"
}`, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "id", "10001"),
				),
			},
		},
	})
}

func TestAccIssueTypeResource_Read_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeState{}

	var readCount atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/issuetype":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Type        string `json:"type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set("10001", body.Name, body.Description, body.Type, 0)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse())

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetype/10001":
			readCount.Add(1)
			if readCount.Load() <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(state.apiResponse())
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/issuetype/10001":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "standard"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", name),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "standard"
}`, name),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccIssueTypeResource_Import(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	state := &issueTypeState{}

	mockServer := newIssueTypeMockServer(state, nil)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_issue_type" "test" {
  name = %q
  description = "Import test"
  type = "standard"
}`, name),
			},
			{
				ResourceName:      "atlassian_jira_issue_type.test",
				ImportState:       true,
				ImportStateId:     "10001",
				ImportStateVerify: true,
			},
		},
	})
}
