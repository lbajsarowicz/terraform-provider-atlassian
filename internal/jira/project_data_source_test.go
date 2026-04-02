package jira_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestAccProjectDataSource_basic(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/project/DSPROJ" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newProjectMockResponse(
				"10010", "DSPROJ", "Data Source Project", "", "software", "ds-lead-123", "UNASSIGNED",
			))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "atlassian_jira_project" "test" { key = "DSPROJ" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_project.test", "id", "10010"),
					resource.TestCheckResourceAttr("data.atlassian_jira_project.test", "key", "DSPROJ"),
					resource.TestCheckResourceAttr("data.atlassian_jira_project.test", "name", "Data Source Project"),
					resource.TestCheckResourceAttr("data.atlassian_jira_project.test", "project_type_key", "software"),
					resource.TestCheckResourceAttr("data.atlassian_jira_project.test", "lead_account_id", "ds-lead-123"),
				),
			},
		},
	})
}

func TestAccProjectDataSource_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "atlassian_jira_project" "test" { key = "NOPE" }`,
				ExpectError: regexp.MustCompile("Project not found"),
			},
		},
	})
}
