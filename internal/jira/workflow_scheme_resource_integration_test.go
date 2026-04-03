package jira_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_project_workflow_scheme", &resource.Sweeper{
		Name: "atlassian_jira_project_workflow_scheme",
		F:    sweepProjectWorkflowSchemes,
	})
	resource.AddTestSweepers("atlassian_jira_workflow_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_workflow_scheme",
		Dependencies: []string{"atlassian_jira_project_workflow_scheme"},
		F:            sweepWorkflowSchemes,
	})
}

func sweepProjectWorkflowSchemes(_ string) error {
	// Project workflow scheme delete is a no-op in the provider.
	// Associations are cleaned up when projects are deleted.
	return nil
}

func sweepWorkflowSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/workflowscheme")
	if err != nil {
		return fmt.Errorf("listing workflow schemes for sweep: %w", err)
	}

	for _, raw := range allValues {
		var scheme struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &scheme); err != nil {
			continue
		}
		if !strings.HasPrefix(scheme.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/workflowscheme/%d", scheme.ID)
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete workflow scheme %q (%d): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationWorkflowSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "Integration test workflow scheme (run %s)"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_workflow_scheme.test", "id"),
				),
			},
			{
				ResourceName:            "atlassian_jira_workflow_scheme.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"issue_type_mappings"},
			},
		},
	})
}

func TestIntegrationWorkflowSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "original"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", rName),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "updated"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationWorkflowSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name = %q
}

data "atlassian_jira_workflow_scheme" "test" {
  name = atlassian_jira_workflow_scheme.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_workflow_scheme.test", "id"),
				),
			},
		},
	})
}

func testCheckWorkflowSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_workflow_scheme" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/workflowscheme/%s", atlassian.PathEscape(rs.Primary.ID)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking workflow scheme %s destruction: %w", rs.Primary.ID, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("workflow scheme %s still exists (status %d)", rs.Primary.ID, statusCode)
		}
	}
	return nil
}
