package confluence_test

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
	resource.AddTestSweepers("atlassian_confluence_space", &resource.Sweeper{
		Name: "atlassian_confluence_space",
		F:    sweepConfluenceSpaces,
	})
}

func sweepConfluenceSpaces(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPagesCursor(ctx, "/wiki/api/v2/spaces?type=global&limit=50")
	if err != nil {
		return fmt.Errorf("listing Confluence spaces for sweep: %w", err)
	}

	for _, raw := range allValues {
		var space struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &space); err != nil {
			continue
		}
		if !strings.HasPrefix(space.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/wiki/rest/api/space/%s", atlassian.PathEscape(space.Key))
		httpResp, delErr := client.Do(ctx, "DELETE", delPath, nil)
		if httpResp != nil {
			httpResp.Body.Close()
		}
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete Confluence space %q (%s): %s\n", space.Name, space.Key, delErr)
		}
	}

	return nil
}

// newSpaceTestIDs returns a random space key (all uppercase letters) and display name
// for use in integration tests.
//
// Confluence space keys must be uppercase letters only. We generate an 8-character
// uppercase suffix and build a key like "TF" + suffix, and a name like "tf-acc-test-" + lowerSuffix.
func newSpaceTestIDs() (key, name string) {
	suffix := acctest.RandStringFromCharSet(8, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	return "TF" + suffix, "tf-acc-test-" + strings.ToLower(suffix)
}

func TestIntegrationConfluenceSpaceResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	rKey, rName := newSpaceTestIDs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckConfluenceSpaceDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testConfluenceSpaceConfig(rKey, rName, "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_confluence_space.test", "key", rKey),
					resource.TestCheckResourceAttr("atlassian_confluence_space.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_confluence_space.test", "description", "Initial description"),
					resource.TestCheckResourceAttrSet("atlassian_confluence_space.test", "id"),
				),
			},
			// Update: change name and description (key is ForceNew so stays the same).
			{
				Config: testConfluenceSpaceConfig(rKey, rName+" updated", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_confluence_space.test", "key", rKey),
					resource.TestCheckResourceAttr("atlassian_confluence_space.test", "name", rName+" updated"),
					resource.TestCheckResourceAttr("atlassian_confluence_space.test", "description", "Updated description"),
				),
			},
			// Import by ID.
			{
				ResourceName:      "atlassian_confluence_space.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationConfluenceSpaceDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	rKey, rName := newSpaceTestIDs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckConfluenceSpaceDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testConfluenceSpaceConfig(rKey, rName, "DS test") + `
data "atlassian_confluence_space" "test" {
  key = atlassian_confluence_space.test.key
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_confluence_space.test", "key", rKey),
					resource.TestCheckResourceAttr("data.atlassian_confluence_space.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_confluence_space.test", "id"),
				),
			},
		},
	})
}

func testConfluenceSpaceConfig(key, name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_confluence_space" "test" {
  key         = %q
  name        = %q
  description = %q
}
`, key, name, description)
}

func testCheckConfluenceSpaceDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_confluence_space" {
			continue
		}
		spaceID := rs.Primary.ID
		apiPath := fmt.Sprintf("/wiki/api/v2/spaces/%s", atlassian.PathEscape(spaceID))

		var result struct {
			ID string `json:"id"`
		}
		statusCode, err := client.GetWithStatus(ctx, apiPath, &result)
		if err != nil {
			// Error likely means 404 / not found; treat as destroyed.
			continue
		}
		if statusCode == 200 {
			return fmt.Errorf("Confluence space %s (id=%s) still exists after destroy", rs.Primary.Attributes["key"], spaceID)
		}
	}
	return nil
}
