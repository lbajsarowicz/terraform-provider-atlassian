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
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_screen_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_screen_scheme",
		Dependencies: []string{"atlassian_jira_issue_type_screen_scheme"},
		F:            sweepScreenSchemes,
	})
}

func sweepScreenSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/screenscheme")
	if err != nil {
		return fmt.Errorf("listing screen schemes for sweep: %w", err)
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

		delPath := fmt.Sprintf("/rest/api/3/screenscheme/%d", scheme.ID)
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete screen scheme %q (%d): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationScreenSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_scheme.test", "id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_scheme.test", "screens.default"),
				),
			},
			{
				ResourceName:      "atlassian_jira_screen_scheme.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationScreenSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "name", rName),
				),
			},
			{
				Config: testIntegrationScreenSchemeConfig(rNameUpdated, "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_screen_scheme.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationScreenSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenSchemeConfig(rName, "ds-test") + `
data "atlassian_jira_screen_scheme" "test" {
  name = atlassian_jira_screen_scheme.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_screen_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_screen_scheme.test", "id"),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_screen_scheme.test", "screens.default"),
				),
			},
		},
	})
}

// testIntegrationScreenSchemeConfig creates a screen and a screen scheme that references it.
func testIntegrationScreenSchemeConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_screen" "ss_dep" {
  name = "%s-screen"
}

resource "atlassian_jira_screen_scheme" "test" {
  name        = %q
  description = %q
  screens = {
    default = atlassian_jira_screen.ss_dep.id
  }
}
`, name, name, description)
}

func testCheckScreenSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_screen_scheme" {
			continue
		}
		// Screen scheme uses GetAllPages — check if ID still exists.
		allValues, err := client.GetAllPages(ctx, "/rest/api/3/screenscheme")
		if err != nil {
			return fmt.Errorf("error listing screen schemes: %w", err)
		}
		for _, raw := range allValues {
			var scheme struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &scheme); err != nil {
				continue
			}
			if fmt.Sprintf("%d", scheme.ID) == rs.Primary.ID {
				return fmt.Errorf("screen scheme %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
