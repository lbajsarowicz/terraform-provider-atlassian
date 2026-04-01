package jira_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
	projectName := "My Test Project"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10001", "PROJ", projectName, "A test project", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/PROJ":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10001", "PROJ", projectName, "A test project", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/project/PROJ":
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
resource "atlassian_jira_project" "test" {
  key              = "PROJ"
  name             = "My Test Project"
  description      = "A test project"
  project_type_key = "software"
  lead_account_id  = "abc123"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "id", "10001"),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "key", "PROJ"),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", "My Test Project"),
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
	updatedName := false

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10002", "UPD", "Original Name", "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/project/UPD":
			updatedName = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10002", "UPD", "Updated Name", "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/UPD":
			w.Header().Set("Content-Type", "application/json")
			name := "Original Name"
			if updatedName {
				name = "Updated Name"
			}
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10002", "UPD", name, "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/project/UPD":
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
resource "atlassian_jira_project" "test" {
  key              = "UPD"
  name             = "Original Name"
  project_type_key = "software"
  lead_account_id  = "abc123"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", "Original Name"),
				),
			},
			{
				Config: `
resource "atlassian_jira_project" "test" {
  key              = "UPD"
  name             = "Updated Name"
  project_type_key = "software"
  lead_account_id  = "abc123"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", "Updated Name"),
				),
			},
		},
	})
}

func TestAccProjectResource_Read_NotFound(t *testing.T) {
	readCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10003", "GONE", "Vanishing Project", "", "software", "abc123", "UNASSIGNED",
			))
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/GONE":
			readCount++
			if readCount <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(newProjectMockResponse(
					"10003", "GONE", "Vanishing Project", "", "software", "abc123", "UNASSIGNED",
				))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/project/GONE":
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
resource "atlassian_jira_project" "test" {
  key              = "GONE"
  name             = "Vanishing Project"
  project_type_key = "software"
  lead_account_id  = "abc123"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", "Vanishing Project"),
				),
			},
			{
				Config: `
resource "atlassian_jira_project" "test" {
  key              = "GONE"
  name             = "Vanishing Project"
  project_type_key = "software"
  lead_account_id  = "abc123"
}`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccProjectResource_Import(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/project":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10004", "IMP", "Import Project", "Imported desc", "business", "lead456", "PROJECT_LEAD",
			))
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/IMP":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10004", "IMP", "Import Project", "Imported desc", "business", "lead456", "PROJECT_LEAD",
			))
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/project/IMP":
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
resource "atlassian_jira_project" "test" {
  key              = "IMP"
  name             = "Import Project"
  description      = "Imported desc"
  project_type_key = "business"
  lead_account_id  = "lead456"
  assignee_type    = "PROJECT_LEAD"
}`,
			},
			{
				ResourceName:      "atlassian_jira_project.test",
				ImportState:       true,
				ImportStateId:     "IMP",
				ImportStateVerify: true,
			},
		},
	})
}
