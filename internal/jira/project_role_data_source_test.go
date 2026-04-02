package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestAccProjectRoleDataSource_basic(t *testing.T) {
	roleName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 10200, "name": "Administrators", "description": "Admin role"},
				{"id": 10201, "name": roleName, "description": "Test role"},
				{"id": 10202, "name": "Developers", "description": "Dev role"},
			})
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
				Config: fmt.Sprintf(`data "atlassian_jira_project_role" "test" { name = %q }`, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_project_role.test", "id", "10201"),
					resource.TestCheckResourceAttr("data.atlassian_jira_project_role.test", "name", roleName),
					resource.TestCheckResourceAttr("data.atlassian_jira_project_role.test", "description", "Test role"),
				),
			},
		},
	})
}

func TestAccProjectRoleDataSource_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 10200, "name": "Administrators", "description": "Admin role"},
			})
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
				Config:      `data "atlassian_jira_project_role" "test" { name = "NonExistentRole" }`,
				ExpectError: regexp.MustCompile("Project role not found"),
			},
		},
	})
}
