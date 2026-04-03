package jira_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_workflow", &resource.Sweeper{
		Name:         "atlassian_jira_workflow",
		Dependencies: []string{"atlassian_jira_workflow_scheme"},
		F:            sweepWorkflows,
	})
}

func sweepWorkflows(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/workflow/search")
	if err != nil {
		return fmt.Errorf("listing workflows for sweep: %w", err)
	}

	for _, raw := range allValues {
		var wf struct {
			ID struct {
				EntityID string `json:"entityId"`
			} `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &wf); err != nil {
			continue
		}
		if !strings.HasPrefix(wf.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/workflow?workflowId=%s", atlassian.QueryEscape(wf.ID.EntityID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete workflow %q (%s): %s\n", wf.Name, wf.ID.EntityID, delErr)
		}
	}

	return nil
}

func TestIntegrationWorkflowResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationWorkflowConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_workflow.test", "id"),
				),
			},
			{
				ResourceName:            "atlassian_jira_workflow.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"statuses"},
			},
		},
	})
}

func TestIntegrationWorkflowDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationWorkflowConfig(rName) + `
data "atlassian_jira_workflow" "test" {
  name = atlassian_jira_workflow.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_workflow.test", "id"),
				),
			},
		},
	})
}

// testIntegrationWorkflowConfig creates a status dependency and a workflow that references it.
func testIntegrationWorkflowConfig(name string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_status" "wf_dep" {
  name            = "%s-status"
  description     = "Workflow dep status (run %s)"
  status_category = "TODO"
}

resource "atlassian_jira_workflow" "test" {
  name        = %q
  description = "Integration test workflow (run %s)"
  statuses    = [atlassian_jira_status.wf_dep.id]
}
`, name, testutil.RunID(), name, testutil.RunID())
}

func testCheckWorkflowDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_workflow" {
			continue
		}
		var result struct {
			Values []struct {
				ID struct {
					EntityID string `json:"entityId"`
				} `json:"id"`
			} `json:"values"`
		}
		searchPath := fmt.Sprintf("/rest/api/3/workflow/search?workflowName=%s",
			atlassian.QueryEscape(rs.Primary.Attributes["name"]))
		if err := client.Get(ctx, searchPath, &result); err != nil {
			return fmt.Errorf("error checking workflow %s destruction: %w", rs.Primary.ID, err)
		}
		for _, wf := range result.Values {
			if wf.ID.EntityID == rs.Primary.ID {
				return fmt.Errorf("workflow %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
