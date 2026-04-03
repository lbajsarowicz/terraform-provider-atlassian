package jira_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestIntegrationPermissionSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckPermissionSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "DS integration test (run %s)"
}

data "atlassian_jira_permission_scheme" "test" {
  name = atlassian_jira_permission_scheme.test.name
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.atlassian_jira_permission_scheme.test", "id",
						"atlassian_jira_permission_scheme.test", "id",
					),
					resource.TestCheckResourceAttr("data.atlassian_jira_permission_scheme.test", "name", rName),
				),
			},
		},
	})
}

func TestIntegrationScreenDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_screen" "test" {
  name        = %q
  description = "DS integration test (run %s)"
}

data "atlassian_jira_screen" "test" {
  name = atlassian_jira_screen.test.name
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.atlassian_jira_screen.test", "id",
						"atlassian_jira_screen.test", "id",
					),
					resource.TestCheckResourceAttr("data.atlassian_jira_screen.test", "name", rName),
				),
			},
		},
	})
}
