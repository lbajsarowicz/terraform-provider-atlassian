package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func newProjectIssueTypeSchemeMockServer(projectID, schemeID string) *httptest.Server {
	var mu sync.Mutex
	currentSchemeID := schemeID

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Assign scheme to project (create or revert on destroy)
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescheme/project":
			var body struct {
				IssueTypeSchemeID string `json:"issueTypeSchemeId"`
				ProjectID         string `json:"projectId"`
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
			currentSchemeID = body.IssueTypeSchemeID
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		// Read: list schemes for project
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme/project":
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
					"issueTypeScheme": map[string]interface{}{
						"id":          sid,
						"name":        "Test Scheme",
						"description": "",
						"isDefault":   false,
					},
					"projectIds": []string{projectID},
				},
			}))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccProjectIssueTypeSchemeResource_basic(t *testing.T) {
	projectID := "10300"
	schemeID := "10100"

	var mu sync.Mutex
	currentSchemeID := schemeID

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescheme/project":
			var body struct {
				IssueTypeSchemeID string `json:"issueTypeSchemeId"`
				ProjectID         string `json:"projectId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentSchemeID = body.IssueTypeSchemeID
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme/project":
			mu.Lock()
			sid := currentSchemeID
			mu.Unlock()
			json.NewEncoder(w).Encode(pageResponse([]interface{}{ //nolint:errcheck
				map[string]interface{}{
					"issueTypeScheme": map[string]interface{}{
						"id": sid, "name": "Test Scheme", "description": "", "isDefault": false,
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
			mu.Lock()
			sid := currentSchemeID
			mu.Unlock()
			if sid != "10000" {
				return fmt.Errorf("expected scheme to revert to default 10000, got %s", sid)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_project_issue_type_scheme" "test" {
  project_id           = %q
  issue_type_scheme_id = %q
}`, projectID, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_issue_type_scheme.test", "project_id", projectID),
					resource.TestCheckResourceAttr("atlassian_jira_project_issue_type_scheme.test", "issue_type_scheme_id", schemeID),
				),
			},
		},
	})
}

func TestAccProjectIssueTypeSchemeResource_import(t *testing.T) {
	projectID := "10301"
	schemeID := "10101"

	mockServer := newProjectIssueTypeSchemeMockServer(projectID, schemeID)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_issue_type_scheme" "test" {
  project_id           = %q
  issue_type_scheme_id = %q
}`, projectID, schemeID),
			},
			{
				ResourceName:                         "atlassian_jira_project_issue_type_scheme.test",
				ImportState:                          true,
				ImportStateId:                        projectID,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_id",
			},
		},
	})
}

func TestAccProjectIssueTypeSchemeResource_notFound(t *testing.T) {
	projectID := "10302"
	schemeID := "10102"

	var mu sync.Mutex
	found := true

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/issuetypescheme/project":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme/project":
			mu.Lock()
			f := found
			mu.Unlock()
			if !f {
				json.NewEncoder(w).Encode(pageResponse([]interface{}{}))
				return
			}
			json.NewEncoder(w).Encode(pageResponse([]interface{}{
				map[string]interface{}{
					"issueTypeScheme": map[string]interface{}{
						"id":          schemeID,
						"name":        "Test Scheme",
						"description": "",
						"isDefault":   false,
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_issue_type_scheme" "test" {
  project_id           = %q
  issue_type_scheme_id = %q
}`, projectID, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_issue_type_scheme.test", "project_id", projectID),
				),
			},
			{
				// Remove from mock state to simulate out-of-band deletion
				PreConfig: func() {
					mu.Lock()
					found = false
					mu.Unlock()
				},
				Config: fmt.Sprintf(`resource "atlassian_jira_project_issue_type_scheme" "test" {
  project_id           = %q
  issue_type_scheme_id = %q
}`, projectID, schemeID),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
