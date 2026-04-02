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

func TestAccIssueTypeDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetype" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "10001",
					"name":           name,
					"description":    "A standard issue type",
					"subtask":        false,
					"hierarchyLevel": 0,
					"avatarId":       10300,
				},
				{
					"id":             "10002",
					"name":           "Sub-task",
					"description":    "A subtask type",
					"subtask":        true,
					"hierarchyLevel": -1,
					"avatarId":       10301,
				},
			})
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
				Config: fmt.Sprintf(`data "atlassian_jira_issue_type" "test" { name = %q }`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "id", "10001"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "description", "A standard issue type"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "type", "standard"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "hierarchy_level", "0"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "avatar_id", "10300"),
				),
			},
		},
	})
}

func TestAccIssueTypeDataSource_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetype" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "10001",
					"name":           "Bug",
					"description":    "A bug",
					"subtask":        false,
					"hierarchyLevel": 0,
					"avatarId":       10300,
				},
			})
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
				Config:      fmt.Sprintf(`data "atlassian_jira_issue_type" "test" { name = %q }`, name),
				ExpectError: regexp.MustCompile("Issue type not found"),
			},
		},
	})
}
