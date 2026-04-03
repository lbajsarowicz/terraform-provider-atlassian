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
	resource.AddTestSweepers("atlassian_jira_project_role_actor", &resource.Sweeper{
		Name: "atlassian_jira_project_role_actor",
		// Actors are cleaned up when the parent project or role is deleted.
		F: func(_ string) error { return nil },
	})
	resource.AddTestSweepers("atlassian_jira_project_role", &resource.Sweeper{
		Name:         "atlassian_jira_project_role",
		Dependencies: []string{"atlassian_jira_project_role_actor"},
		F:            sweepProjectRoles,
	})
}

func sweepProjectRoles(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// GET /rest/api/3/role returns a flat JSON array (not the paginated "values" shape).
	var roles []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := client.Get(ctx, "/rest/api/3/role", &roles); err != nil {
		return fmt.Errorf("listing project roles for sweep: %w", err)
	}

	for _, role := range roles {
		if !strings.HasPrefix(role.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/role/%d", role.ID)
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete project role %q (%d): %s\n", role.Name, role.ID, delErr)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Project Role resource tests
// ---------------------------------------------------------------------------

func TestIntegrationProjectRoleResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectRoleConfig(rName, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "description", ""),
					resource.TestCheckResourceAttrSet("atlassian_jira_project_role.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_project_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationProjectRoleResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-updated"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectRoleConfig(rName, "initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "description", "initial description"),
				),
			},
			{
				Config: testIntegrationProjectRoleConfig(rNameUpdated, "updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "description", "updated description"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Project Role data source test
// ---------------------------------------------------------------------------

func TestIntegrationProjectRoleDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_project_role" "test" {
  name        = %q
  description = "data source test role"
}

data "atlassian_jira_project_role" "test" {
  name = atlassian_jira_project_role.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_project_role.test", "name", rName),
					resource.TestCheckResourceAttr("data.atlassian_jira_project_role.test", "description", "data source test role"),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_project_role.test", "id"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Project Role Actor resource test
// ---------------------------------------------------------------------------

func TestIntegrationProjectRoleActorResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	rSuffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rKey := "TFACC" + rSuffix
	rName := acctest.RandomWithPrefix("tf-acc-test")
	leadAccountID := testIntegrationProjectLeadAccountID(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectRoleActorProjectDestroyed(rKey),
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectRoleActorConfig(rKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_project_role_actor.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "project_key", rKey),
					resource.TestCheckResourceAttrSet("atlassian_jira_project_role_actor.test", "role_id"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_type", "atlassianGroup"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_value", rName),
				),
			},
			{
				ResourceName:      "atlassian_jira_project_role_actor.test",
				ImportState:       true,
				ImportStateIdFunc: testProjectRoleActorImportID("atlassian_jira_project_role_actor.test"),
				ImportStateVerify: true,
			},
		},
	})
}

// testProjectRoleActorImportID builds the composite import ID
// "{project_key}/{role_id}/{actor_type}/{actor_value}" from state.
func testProjectRoleActorImportID(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceAddr)
		}
		projectKey := rs.Primary.Attributes["project_key"]
		roleID := rs.Primary.Attributes["role_id"]
		actorType := rs.Primary.Attributes["actor_type"]
		actorValue := rs.Primary.Attributes["actor_value"]
		return fmt.Sprintf("%s/%s/%s/%s", projectKey, roleID, actorType, actorValue), nil
	}
}

// ---------------------------------------------------------------------------
// Destroy checks
// ---------------------------------------------------------------------------

func testCheckProjectRoleDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_project_role" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/role/%s", atlassian.PathEscape(rs.Primary.ID)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking project role %s destruction: %w", rs.Primary.ID, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("project role %s still exists (status %d)", rs.Primary.ID, statusCode)
		}
	}
	return nil
}

// testCheckProjectRoleActorProjectDestroyed checks that the project used in the actor
// test has been deleted (actors are destroyed when the project is deleted).
func testCheckProjectRoleActorProjectDestroyed(projectKey string) func(*terraform.State) error {
	return func(s *terraform.State) error {
		client, err := testutil.SweepClient()
		if err != nil {
			return fmt.Errorf("creating client for destroy check: %w", err)
		}
		ctx := context.Background()

		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(projectKey)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking project %s destruction: %w", projectKey, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("project %s still exists (status %d)", projectKey, statusCode)
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// Config helpers
// ---------------------------------------------------------------------------

func testIntegrationProjectRoleConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_project_role" "test" {
  name        = %q
  description = %q
}
`, name, description)
}

func testIntegrationProjectRoleActorConfig(projectKey, name, leadAccountID string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_group" "test" {
  name = %q
}

resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = %q
  description      = "Integration test project for role actor (run %s)"
}

resource "atlassian_jira_project_role" "test" {
  name        = %q
  description = "Integration test role for actor"
}

resource "atlassian_jira_project_role_actor" "test" {
  project_key = atlassian_jira_project.test.key
  role_id     = atlassian_jira_project_role.test.id
  actor_type  = "atlassianGroup"
  actor_value = atlassian_jira_group.test.name
}
`, name, projectKey, name, leadAccountID, testutil.RunID(), name)
}

// sweepProjectRolesViaJSON is a helper used only to satisfy the JSON unmarshal
// pattern; it is kept unexported to document the flat-array API shape.
var _ = json.Unmarshal // ensure encoding/json import is used
