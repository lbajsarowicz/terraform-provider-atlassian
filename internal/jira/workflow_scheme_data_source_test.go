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

func TestAccWorkflowSchemeDataSource_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/workflowscheme" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"id":                10001,
						"name":              "Default Workflow Scheme",
						"description":       "Default",
						"defaultWorkflow":   "jira",
						"issueTypeMappings": map[string]string{},
						"draft":             false,
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
				Config:      `data "atlassian_jira_workflow_scheme" "test" { name = "NonExistentWorkflowScheme" }`,
				ExpectError: regexp.MustCompile("Workflow scheme not found"),
			},
		},
	})
}
