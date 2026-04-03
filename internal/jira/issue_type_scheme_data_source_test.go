package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestAccIssueTypeSchemeDataSource_found(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"id":          "10001",
						"name":        name,
						"description": "Test issue type scheme",
						"isDefault":   false,
					},
				},
				"startAt":    0,
				"maxResults": 50,
				"total":      1,
				"isLast":     true,
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme/mapping" &&
			strings.Contains(r.URL.RawQuery, "issueTypeSchemeId=10001"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"issueTypeSchemeId": "10001", "issueTypeId": "10100"},
					{"issueTypeSchemeId": "10001", "issueTypeId": "10101"},
				},
				"startAt":    0,
				"maxResults": 50,
				"total":      2,
				"isLast":     true,
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
				Config: fmt.Sprintf(`data "atlassian_jira_issue_type_scheme" "test" { name = %q }`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "id", "10001"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "description", "Test issue type scheme"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "issue_type_ids.#", "2"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "issue_type_ids.0", "10100"),
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "issue_type_ids.1", "10101"),
				),
			},
		},
	})
}

func TestAccIssueTypeSchemeDataSource_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/issuetypescheme" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"id":          "10001",
						"name":        "Default Issue Type Scheme",
						"description": "The default",
						"isDefault":   true,
					},
				},
				"startAt":    0,
				"maxResults": 50,
				"total":      1,
				"isLast":     true,
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
				Config:      `data "atlassian_jira_issue_type_scheme" "test" { name = "NonExistentScheme" }`,
				ExpectError: regexp.MustCompile("Issue type scheme not found"),
			},
		},
	})
}
