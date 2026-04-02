package jira_test

import (
	"context"
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
	resource.AddTestSweepers("atlassian_jira_project_permission_scheme", &resource.Sweeper{
		Name: "atlassian_jira_project_permission_scheme",
		F:    sweepProjectPermissionSchemes,
	})
	resource.AddTestSweepers("atlassian_jira_permission_scheme_grant", &resource.Sweeper{
		Name:         "atlassian_jira_permission_scheme_grant",
		Dependencies: []string{"atlassian_jira_project_permission_scheme"},
		F:            sweepPermissionSchemeGrants,
	})
	resource.AddTestSweepers("atlassian_jira_permission_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_permission_scheme",
		Dependencies: []string{"atlassian_jira_permission_scheme_grant"},
		F:            sweepPermissionSchemes,
	})
}

func sweepProjectPermissionSchemes(_ string) error {
	// Project permission scheme associations are cleaned up when projects are deleted
	// by the atlassian_jira_project sweeper.
	return nil
}

func sweepPermissionSchemeGrants(_ string) error {
	// Grants are deleted when the parent scheme is deleted.
	return nil
}

func sweepPermissionSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// The permission scheme list endpoint uses a "permissionSchemes" key, NOT the
	// standard "values" key used by GetAllPages. Decode manually.
	var listResp struct {
		PermissionSchemes []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"permissionSchemes"`
	}
	if err := client.Get(ctx, "/rest/api/3/permissionscheme", &listResp); err != nil {
		return fmt.Errorf("listing permission schemes for sweep: %w", err)
	}

	for _, scheme := range listResp.PermissionSchemes {
		if !strings.HasPrefix(scheme.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/permissionscheme/%d", scheme.ID)
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete permission scheme %q (%d): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationPermissionSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckPermissionSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationPermissionSchemeConfig(rName, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_permission_scheme.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_permission_scheme.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationPermissionSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckPermissionSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationPermissionSchemeConfig(rName, "initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "description", "initial description"),
				),
			},
			{
				Config: testIntegrationPermissionSchemeConfig(rNameUpdated, "updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "description", "updated description"),
				),
			},
		},
	})
}

func TestIntegrationPermissionSchemeGrantResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckPermissionSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationPermissionSchemeGrantConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_permission_scheme.test", "id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_permission_scheme_grant.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_type", "anyone"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "permission", "BROWSE_PROJECTS"),
				),
			},
		},
	})
}

func testCheckPermissionSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_permission_scheme" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/permissionscheme/%s", atlassian.PathEscape(rs.Primary.ID)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking permission scheme %s destruction: %w", rs.Primary.ID, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("permission scheme %s still exists (status %d)", rs.Primary.ID, statusCode)
		}
	}
	return nil
}

func testIntegrationPermissionSchemeConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = %q
}
`, name, description)
}

func testIntegrationPermissionSchemeGrantConfig(name string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Integration test permission scheme (run %s)"
}

resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id   = atlassian_jira_permission_scheme.test.id
  permission  = "BROWSE_PROJECTS"
  holder_type = "anyone"
}
`, name, testutil.RunID())
}
