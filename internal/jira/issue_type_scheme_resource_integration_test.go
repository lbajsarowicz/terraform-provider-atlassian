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
	resource.AddTestSweepers("atlassian_jira_project_issue_type_scheme", &resource.Sweeper{
		Name: "atlassian_jira_project_issue_type_scheme",
		F:    sweepProjectIssueTypeSchemes,
	})
	resource.AddTestSweepers("atlassian_jira_issue_type_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_issue_type_scheme",
		Dependencies: []string{"atlassian_jira_project_issue_type_scheme"},
		F:            sweepIssueTypeSchemes,
	})
}

func sweepProjectIssueTypeSchemes(_ string) error {
	// Project issue type scheme associations are cleaned up when projects are deleted
	// by the atlassian_jira_project sweeper, or when schemes are deleted.
	return nil
}

func sweepIssueTypeSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/issuetypescheme")
	if err != nil {
		return fmt.Errorf("listing issue type schemes for sweep: %w", err)
	}

	for _, raw := range allValues {
		var scheme struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &scheme); err != nil {
			continue
		}
		if !strings.HasPrefix(scheme.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s", atlassian.PathEscape(scheme.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete issue type scheme %q (%s): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationIssueTypeSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_issue_type_scheme.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "issue_type_ids.#", "1"),
				),
			},
			{
				ResourceName:      "atlassian_jira_issue_type_scheme.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationIssueTypeSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", rName),
				),
			},
			{
				Config: testIntegrationIssueTypeSchemeConfig(rNameUpdated, "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationIssueTypeSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeSchemeConfig(rName, "ds-test") + `
data "atlassian_jira_issue_type_scheme" "test" {
  name = atlassian_jira_issue_type_scheme.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_issue_type_scheme.test", "id"),
				),
			},
		},
	})
}

// testIntegrationIssueTypeSchemeConfig creates an issue type and an issue type scheme.
func testIntegrationIssueTypeSchemeConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_issue_type" "its_dep" {
  name = "%s-issuetype"
  type = "standard"
}

resource "atlassian_jira_issue_type_scheme" "test" {
  name           = %q
  description    = %q
  issue_type_ids = [atlassian_jira_issue_type.its_dep.id]
}
`, name, name, description)
}

func testCheckIssueTypeSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_issue_type_scheme" {
			continue
		}
		allValues, err := client.GetAllPages(ctx, "/rest/api/3/issuetypescheme")
		if err != nil {
			return fmt.Errorf("error listing issue type schemes: %w", err)
		}
		for _, raw := range allValues {
			var scheme struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &scheme); err != nil {
				continue
			}
			if scheme.ID == rs.Primary.ID {
				return fmt.Errorf("issue type scheme %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
