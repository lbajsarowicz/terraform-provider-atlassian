package jira_test

import (
	"context"
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
	resource.AddTestSweepers("atlassian_jira_custom_field", &resource.Sweeper{
		Name: "atlassian_jira_custom_field",
		F:    sweepCustomFields,
	})
}

func sweepCustomFields(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// GET /rest/api/3/field returns a flat array of all fields (not paginated).
	var fields []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Custom bool   `json:"custom"`
	}
	if err := client.Get(ctx, "/rest/api/3/field", &fields); err != nil {
		return fmt.Errorf("listing fields for sweep: %w", err)
	}

	for _, f := range fields {
		if !f.Custom || !strings.HasPrefix(f.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/field/%s", atlassian.PathEscape(f.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete custom field %q (%s): %s\n", f.Name, f.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationCustomFieldResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckCustomFieldDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "Integration test custom field (run %s)"
  type        = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_custom_field.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "type", "com.atlassian.jira.plugin.system.customfieldtypes:textfield"),
				),
			},
			{
				ResourceName:      "atlassian_jira_custom_field.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationCustomFieldResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckCustomFieldDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "original"
  type        = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
}
`, rName),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "updated"
  type        = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationCustomFieldDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckCustomFieldDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_custom_field" "test" {
  name = %q
  type = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
}

data "atlassian_jira_custom_field" "test" {
  name = atlassian_jira_custom_field.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_custom_field.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_custom_field.test", "id"),
				),
			},
		},
	})
}

func testCheckCustomFieldDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()

	var fields []struct {
		ID     string `json:"id"`
		Custom bool   `json:"custom"`
	}
	if err := client.Get(ctx, "/rest/api/3/field", &fields); err != nil {
		return fmt.Errorf("error listing fields for destroy check: %w", err)
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_custom_field" {
			continue
		}
		for _, f := range fields {
			if f.ID == rs.Primary.ID {
				return fmt.Errorf("custom field %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
