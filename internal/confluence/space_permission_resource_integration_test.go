package confluence_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_confluence_space_permission", &resource.Sweeper{
		Name:         "atlassian_confluence_space_permission",
		Dependencies: []string{"atlassian_confluence_space"},
		F:            sweepConfluenceSpacePermissions,
	})
}

func sweepConfluenceSpacePermissions(_ string) error {
	// Permissions are swept indirectly through space deletion; this sweeper
	// exists to satisfy the dependency declaration.
	_ = testutil.AllowedSweepHosts // ensure testutil is used
	return nil
}

// getAtlassianUserAccountID fetches the account ID of the authenticated API user.
// This is used in tests to grant permissions to the test user.
func getAtlassianUserAccountID(ctx context.Context, client *atlassian.Client) (string, error) {
	var result struct {
		AccountID string `json:"accountId"`
	}
	if err := client.Get(ctx, "/rest/api/3/myself", &result); err != nil {
		return "", fmt.Errorf("fetching current user: %w", err)
	}
	return result.AccountID, nil
}

func TestIntegrationConfluenceSpacePermissionResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	testutil.SkipIfNoConfluencePermissions(t)

	rKey, rName := newSpaceTestIDs()

	// Get the account ID of the API user to use as permission subject.
	client, err := testutil.SweepClient()
	if err != nil {
		t.Fatalf("creating client: %s", err)
	}
	accountID, err := getAtlassianUserAccountID(context.Background(), client)
	if err != nil {
		t.Fatalf("getting account ID: %s", err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckConfluenceSpacePermissionDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testConfluenceSpacePermissionConfig(rKey, rName, accountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_confluence_space_permission.test", "id"),
					resource.TestCheckResourceAttr("atlassian_confluence_space_permission.test", "space_key", rKey),
					resource.TestCheckResourceAttr("atlassian_confluence_space_permission.test", "principal_type", "user"),
					resource.TestCheckResourceAttr("atlassian_confluence_space_permission.test", "principal_id", accountID),
					resource.TestCheckResourceAttr("atlassian_confluence_space_permission.test", "operation_key", "read"),
					resource.TestCheckResourceAttr("atlassian_confluence_space_permission.test", "operation_target", "space"),
				),
			},
		},
	})
}

func testConfluenceSpacePermissionConfig(key, name, accountID string) string {
	return fmt.Sprintf(`
resource "atlassian_confluence_space" "test" {
  key         = %q
  name        = %q
  description = "Permission test space (run %s)"
}

resource "atlassian_confluence_space_permission" "test" {
  space_key        = atlassian_confluence_space.test.key
  space_id         = atlassian_confluence_space.test.id
  principal_type   = "user"
  principal_id     = %q
  operation_key    = "read"
  operation_target = "space"
}
`, key, name, testutil.RunID(), accountID)
}

func testCheckConfluenceSpacePermissionDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_confluence_space_permission" {
			continue
		}
		spaceID := rs.Primary.Attributes["space_id"]
		permID := rs.Primary.ID
		if spaceID == "" {
			continue
		}

		apiPath := fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", atlassian.PathEscape(spaceID))
		allPerms, permListStatus, err := client.GetAllPagesCursorWithStatus(ctx, apiPath)
		if err != nil {
			return fmt.Errorf("listing permissions for space %s: %w", spaceID, err)
		}
		if permListStatus == 404 {
			// Space already gone; permission implicitly destroyed.
			continue
		}
		for _, raw := range allPerms {
			var perm struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &perm); err != nil {
				continue
			}
			if perm.ID == permID {
				return fmt.Errorf("Confluence space permission %s still exists after destroy", permID)
			}
		}

		// Check space also destroyed
		spaceAPIPath := fmt.Sprintf("/wiki/api/v2/spaces/%s", atlassian.PathEscape(spaceID))
		var spaceResult struct{ ID string `json:"id"` }
		statusCode, _ := client.GetWithStatus(ctx, spaceAPIPath, &spaceResult)
		if statusCode == 200 {
			spaceKey := rs.Primary.Attributes["space_key"]
			return fmt.Errorf("Confluence space %s (id=%s) still exists after destroy", spaceKey, spaceID)
		}
	}
	return nil
}

