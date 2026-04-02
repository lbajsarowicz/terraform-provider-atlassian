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
	resource.AddTestSweepers("atlassian_jira_project", &resource.Sweeper{
		Name:         "atlassian_jira_project",
		Dependencies: []string{"atlassian_jira_group"},
		F:            sweepProjects,
	})
}

func sweepProjects(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/project/search")
	if err != nil {
		return fmt.Errorf("listing projects for sweep: %w", err)
	}

	for _, raw := range allValues {
		var project struct {
			ID   string `json:"id"`
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &project); err != nil {
			continue
		}
		if !strings.HasPrefix(project.Name, "tf-acc-test-") && !strings.HasPrefix(project.Key, "TFACC") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(project.Key))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete project %q (%s): %s\n", project.Name, project.Key, delErr)
		}
	}

	return nil
}

func TestIntegrationProjectResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rSuffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rKey := "TFACC" + rSuffix
	rName := acctest.RandomWithPrefix("tf-acc-test")
	leadAccountID := testIntegrationProjectLeadAccountID(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectConfig(rKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "key", rKey),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_project.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_project.test",
				ImportState:       true,
				ImportStateId:     rKey,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationProjectResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rSuffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rKey := "TFACC" + rSuffix
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-updated"
	leadAccountID := testIntegrationProjectLeadAccountID(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectConfig(rKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", rName),
				),
			},
			{
				Config: testIntegrationProjectConfig(rKey, rNameUpdated, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", rNameUpdated),
				),
			},
		},
	})
}

// testIntegrationProjectLeadAccountID fetches the authenticated user's account ID
// from the Jira "myself" endpoint. This is used as lead_account_id in project tests.
func testIntegrationProjectLeadAccountID(t *testing.T) string {
	t.Helper()
	client, err := testutil.SweepClient()
	if err != nil {
		t.Fatalf("creating client to look up lead account ID: %s", err)
	}
	var myself struct {
		AccountID string `json:"accountId"`
	}
	if err := client.Get(context.Background(), "/rest/api/3/myself", &myself); err != nil {
		t.Fatalf("fetching current user for lead_account_id: %s", err)
	}
	return myself.AccountID
}

func testIntegrationProjectConfig(key, name, leadAccountID string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = %q
  description      = "Integration test project (run %s)"
}
`, key, name, leadAccountID, testutil.RunID())
}

func testCheckProjectDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_project" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(rs.Primary.Attributes["key"])),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking project %s destruction: %w", rs.Primary.Attributes["key"], err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("project %s still exists (status %d)", rs.Primary.Attributes["key"], statusCode)
		}
	}
	return nil
}
