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
	resource.AddTestSweepers("atlassian_jira_project_issue_type_screen_scheme", &resource.Sweeper{
		Name: "atlassian_jira_project_issue_type_screen_scheme",
		F:    sweepProjectIssueTypeScreenSchemes,
	})
	resource.AddTestSweepers("atlassian_jira_issue_type_screen_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_issue_type_screen_scheme",
		Dependencies: []string{"atlassian_jira_project_issue_type_screen_scheme"},
		F:            sweepIssueTypeScreenSchemes,
	})
}

func sweepProjectIssueTypeScreenSchemes(_ string) error {
	// Project associations are cleaned up when projects are deleted or when
	// the issue type screen scheme is deleted.
	return nil
}

func sweepIssueTypeScreenSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/issuetypescreenscheme")
	if err != nil {
		return fmt.Errorf("listing issue type screen schemes for sweep: %w", err)
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

		delPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", atlassian.PathEscape(scheme.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete issue type screen scheme %q (%s): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationIssueTypeScreenSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeScreenSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeScreenSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_issue_type_screen_scheme.test", "id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_issue_type_screen_scheme.test", "mappings.default"),
				),
			},
			{
				ResourceName:            "atlassian_jira_issue_type_screen_scheme.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"mappings"},
			},
		},
	})
}

func TestIntegrationIssueTypeScreenSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeScreenSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeScreenSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "name", rName),
				),
			},
			{
				Config: testIntegrationIssueTypeScreenSchemeConfig(rNameUpdated, "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_screen_scheme.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationIssueTypeScreenSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeScreenSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeScreenSchemeConfig(rName, "ds-test") + `
data "atlassian_jira_issue_type_screen_scheme" "test" {
  name = atlassian_jira_issue_type_screen_scheme.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_screen_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_issue_type_screen_scheme.test", "id"),
				),
			},
		},
	})
}

// testIntegrationIssueTypeScreenSchemeConfig creates a screen → screen scheme → issue type screen scheme chain.
func testIntegrationIssueTypeScreenSchemeConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_screen" "itss_dep" {
  name = "%s-screen"
}

resource "atlassian_jira_screen_scheme" "itss_dep" {
  name = "%s-screenscheme"
  screens = {
    default = atlassian_jira_screen.itss_dep.id
  }
}

resource "atlassian_jira_issue_type_screen_scheme" "test" {
  name        = %q
  description = %q
  mappings = {
    default = atlassian_jira_screen_scheme.itss_dep.id
  }
}
`, name, name, name, description)
}

func testCheckIssueTypeScreenSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_issue_type_screen_scheme" {
			continue
		}
		allValues, err := client.GetAllPages(ctx, "/rest/api/3/issuetypescreenscheme")
		if err != nil {
			return fmt.Errorf("error listing issue type screen schemes: %w", err)
		}
		for _, raw := range allValues {
			var scheme struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &scheme); err != nil {
				continue
			}
			if scheme.ID == rs.Primary.ID {
				return fmt.Errorf("issue type screen scheme %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
