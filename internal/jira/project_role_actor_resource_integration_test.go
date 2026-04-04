package jira_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestIntegrationProjectRoleActorResource_user(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	rName := acctest.RandomWithPrefix("tf-acc-test")
	projectKey := fmt.Sprintf("TFACC%s", acctest.RandStringFromCharSet(5, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))

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
		CheckDestroy:             testCheckProjectRoleActorRemoved,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectRoleActorResourceConfig(projectKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "project_key", projectKey),
					resource.TestCheckResourceAttrSet("atlassian_jira_project_role_actor.test", "role_id"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_type", "atlassianUser"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_value", leadAccountID),
				),
			},
			{
				ResourceName: "atlassian_jira_project_role_actor.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["atlassian_jira_project_role_actor.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					roleID := rs.Primary.Attributes["role_id"]
					actorValue := rs.Primary.Attributes["actor_value"]
					return fmt.Sprintf("%s/%s/atlassianUser/%s", projectKey, roleID, actorValue), nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func testIntegrationProjectRoleActorResourceConfig(projectKey, name, leadAccountID string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = %q
}

resource "atlassian_jira_project_role" "test" {
  name = %q
}

resource "atlassian_jira_project_role_actor" "test" {
  project_key = atlassian_jira_project.test.key
  role_id     = atlassian_jira_project_role.test.id
  actor_type  = "atlassianUser"
  actor_value = %q
}
`, projectKey, name, leadAccountID, name+"-role", leadAccountID)
}

func testCheckProjectRoleActorRemoved(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_project_role_actor" {
			continue
		}
		projectKey := rs.Primary.Attributes["project_key"]
		roleID := rs.Primary.Attributes["role_id"]
		actorType := rs.Primary.Attributes["actor_type"]
		actorValue := rs.Primary.Attributes["actor_value"]

		apiPath := fmt.Sprintf("/rest/api/3/project/%s/role/%s",
			atlassian.PathEscape(projectKey),
			atlassian.PathEscape(roleID),
		)

		var result struct {
			Actors []struct {
				Type       string                                  `json:"type"`
				ActorUser  *struct{ AccountID string `json:"accountId"` } `json:"actorUser"`
				ActorGroup *struct{ Name string `json:"name"` }           `json:"actorGroup"`
			} `json:"actors"`
		}
		statusCode, err := client.GetWithStatus(ctx, apiPath, &result)
		if err != nil {
			return fmt.Errorf("reading project role actors: %w", err)
		}
		if statusCode == 404 {
			continue
		}

		for _, actor := range result.Actors {
			if actorType == "atlassianUser" && actor.Type == "atlassian-user-role-actor" {
				if actor.ActorUser != nil && actor.ActorUser.AccountID == actorValue {
					return fmt.Errorf("actor %s still exists in project %s role %s", actorValue, projectKey, roleID)
				}
			}
			if actorType == "atlassianGroup" && actor.Type == "atlassian-group-role-actor" {
				if actor.ActorGroup != nil && actor.ActorGroup.Name == actorValue {
					return fmt.Errorf("actor %s still exists in project %s role %s", actorValue, projectKey, roleID)
				}
			}
		}
	}
	return nil
}
