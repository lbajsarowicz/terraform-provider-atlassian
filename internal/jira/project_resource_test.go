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

func newProjectMockResponse(id, key, name, description, projectTypeKey, leadAccountID, assigneeType string) map[string]interface{} {
	return map[string]interface{}{
		"id":             id,
		"key":            key,
		"name":           name,
		"description":    description,
		"projectTypeKey": projectTypeKey,
		"lead": map[string]string{
			"accountId": leadAccountID,
		},
		"assigneeType": assigneeType,
	}
}

func TestAccProjectResource_basic(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	projectKey := fmt.Sprintf("T%s", suffix)
	projectName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdProject := map[string]string{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdProject["id"] = "10001"
			createdProject["key"] = body["key"]
			createdProject["name"] = body["name"]
			createdProject["description"] = body["description"]
			createdProject["projectTypeKey"] = body["projectTypeKey"]
			createdProject["leadAccountId"] = body["leadAccountId"]
			createdProject["assigneeType"] = body["assigneeType"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10001", body["key"], body["name"], body["description"],
				body["projectTypeKey"], body["leadAccountId"], body["assigneeType"],
			))
		case r.Method == "GET" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
			mu.Lock()
			p := make(map[string]string)
			for k, v := range createdProject {
				p[k] = v
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				p["id"], p["key"], p["name"], p["description"],
				p["projectTypeKey"], p["leadAccountId"], p["assigneeType"],
			))
		case r.Method == "DELETE" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  description      = "A test project"
  project_type_key = "software"
  lead_account_id  = "abc123"
}`, projectKey, projectName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "id", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "key", projectKey),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", projectName),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "description", "A test project"),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "project_type_key", "software"),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "lead_account_id", "abc123"),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "assignee_type", "UNASSIGNED"),
				),
			},
		},
	})
}

func TestAccProjectResource_update(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	projectKey := fmt.Sprintf("T%s", suffix)
	originalName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	currentName := ""

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentName = body["name"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10002", body["key"], body["name"], "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "PUT" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentName = body["name"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10002", projectKey, body["name"], "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "GET" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
			mu.Lock()
			name := currentName
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10002", projectKey, name, "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "DELETE" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = "abc123"
}`, projectKey, originalName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", originalName),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = "abc123"
}`, projectKey, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", updatedName),
				),
			},
		},
	})
}

func TestAccProjectResource_Read_NotFound(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	projectKey := fmt.Sprintf("T%s", suffix)
	projectName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10003", body["key"], body["name"], "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "GET" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
			readCount.Add(1)
			if readCount.Load() <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(newProjectMockResponse(
					"10003", projectKey, projectName, "", "software", "abc123", "UNASSIGNED",
				))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = "abc123"
}`, projectKey, projectName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", projectName),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = "abc123"
}`, projectKey, projectName),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccProjectResource_Import(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	projectKey := fmt.Sprintf("T%s", suffix)
	projectName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdProject := map[string]string{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdProject["id"] = "10004"
			createdProject["key"] = body["key"]
			createdProject["name"] = body["name"]
			createdProject["description"] = body["description"]
			createdProject["projectTypeKey"] = body["projectTypeKey"]
			createdProject["leadAccountId"] = body["leadAccountId"]
			createdProject["assigneeType"] = body["assigneeType"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10004", body["key"], body["name"], body["description"],
				body["projectTypeKey"], body["leadAccountId"], body["assigneeType"],
			))
		case r.Method == "GET" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
			mu.Lock()
			p := make(map[string]string)
			for k, v := range createdProject {
				p[k] = v
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				p["id"], p["key"], p["name"], p["description"],
				p["projectTypeKey"], p["leadAccountId"], p["assigneeType"],
			))
		case r.Method == "DELETE" && r.URL.Path == fmt.Sprintf("/rest/api/3/project/%s", projectKey):
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  description      = "Imported desc"
  project_type_key = "business"
  lead_account_id  = "lead456"
  assignee_type    = "PROJECT_LEAD"
}`, projectKey, projectName),
			},
			{
				ResourceName:      "atlassian_jira_project.test",
				ImportState:       true,
				ImportStateId:     projectKey,
				ImportStateVerify: true,
			},
		},
	})
}
