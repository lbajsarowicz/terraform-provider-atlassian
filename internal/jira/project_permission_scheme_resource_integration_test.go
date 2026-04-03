package jira_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestIntegrationProjectPermissionSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	rName := acctest.RandomWithPrefix("tf-acc-test")
	projectKey := fmt.Sprintf("TFACC%s", acctest.RandStringFromCharSet(6, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))

	client, err := testutil.SweepClient()
	if err != nil {
		t.Fatalf("creating client: %s", err)
	}
	leadAccountID, err := getTestAccountID(context.Background(), client)
	if err != nil {
		t.Fatalf("getting account ID: %s", err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectPermissionSchemeReverted,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectPermissionSchemeConfig(projectKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "project_key", projectKey),
					resource.TestCheckResourceAttrSet("atlassian_jira_project_permission_scheme.test", "scheme_id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_project_permission_scheme.test",
				ImportState:       true,
				ImportStateId:     projectKey,
				ImportStateVerify: true,
			},
		},
	})
}

func testIntegrationProjectPermissionSchemeConfig(projectKey, name, leadAccountID string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = %q
}

resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Integration test (run %s)"
}

resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = atlassian_jira_project.test.key
  scheme_id   = atlassian_jira_permission_scheme.test.id
}
`, projectKey, name, leadAccountID, name+"-scheme", testutil.RunID())
}

func testCheckProjectPermissionSchemeReverted(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_project_permission_scheme" {
			continue
		}
		testSchemeID := rs.Primary.Attributes["scheme_id"]
		projectKey := rs.Primary.Attributes["project_key"]

		var result struct {
			ID int `json:"id"`
		}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/project/%s/permissionscheme", atlassian.PathEscape(projectKey)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("reading project %s permission scheme: %w", projectKey, err)
		}
		if statusCode == http.StatusNotFound {
			continue
		}
		// Delete must reset to the default permission scheme. We verify the test scheme is
		// no longer assigned but don't assert a specific default ID because it varies per
		// tenant. A future improvement could fetch the list of schemes and verify the
		// assigned one is the first/only built-in scheme.
		currentSchemeID := fmt.Sprintf("%d", result.ID)
		if currentSchemeID == testSchemeID {
			return fmt.Errorf("project %s still has test scheme %s assigned after destroy", projectKey, testSchemeID)
		}
	}
	return nil
}

// getTestAccountID fetches the account ID of the authenticated user.
func getTestAccountID(ctx context.Context, client *atlassian.Client) (string, error) {
	var result struct {
		AccountID string `json:"accountId"`
	}
	if err := client.Get(ctx, "/rest/api/3/myself", &result); err != nil {
		return "", fmt.Errorf("fetching current user: %w", err)
	}
	return result.AccountID, nil
}
