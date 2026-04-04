package jira_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestIntegrationProjectIssueTypeSchemeResource_basic(t *testing.T) {
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
		CheckDestroy:             testCheckProjectIssueTypeSchemeReverted,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectIssueTypeSchemeConfig(projectKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_project_issue_type_scheme.test", "project_id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_project_issue_type_scheme.test", "issue_type_scheme_id"),
				),
			},
			{
				ResourceName:                         "atlassian_jira_project_issue_type_scheme.test",
				ImportState:                          true,
				ImportStateIdFunc:                    testProjectIssueTypeSchemeImportID,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_id",
			},
		},
	})
}

func testProjectIssueTypeSchemeImportID(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources["atlassian_jira_project_issue_type_scheme.test"]
	if !ok {
		return "", fmt.Errorf("resource not found in state")
	}
	return rs.Primary.Attributes["project_id"], nil
}

func testIntegrationProjectIssueTypeSchemeConfig(projectKey, name, leadAccountID string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = %q
}

resource "atlassian_jira_issue_type" "its_dep" {
  name        = %q
  description = "Integration test issue type (run %s)"
  type        = "standard"
}

resource "atlassian_jira_issue_type_scheme" "test" {
  name           = %q
  description    = "Integration test (run %s)"
  issue_type_ids = [atlassian_jira_issue_type.its_dep.id]
}

resource "atlassian_jira_project_issue_type_scheme" "test" {
  project_id           = atlassian_jira_project.test.id
  issue_type_scheme_id = atlassian_jira_issue_type_scheme.test.id
}
`, projectKey, name, leadAccountID, name+"-it", testutil.RunID(), name+"-its", testutil.RunID())
}

func testCheckProjectIssueTypeSchemeReverted(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_project_issue_type_scheme" {
			continue
		}
		testSchemeID := rs.Primary.Attributes["issue_type_scheme_id"]
		projectID := rs.Primary.Attributes["project_id"]
		if projectID == "" {
			continue
		}

		allValues, err := client.GetAllPages(ctx, fmt.Sprintf("/rest/api/3/issuetypescheme/project?projectId=%s", projectID))
		if err != nil {
			return fmt.Errorf("listing issue type scheme for project %s: %w", projectID, err)
		}

		// Delete must revert to the default scheme — verify an association still exists
		// and that it is no longer the test scheme. The Cloud default is typically ID 10000
		// but we don't hardcode it because it varies per tenant.
		found := false
		for _, raw := range allValues {
			var entry struct {
				IssueTypeScheme struct {
					ID string `json:"id"`
				} `json:"issueTypeScheme"`
				ProjectIDs []string `json:"projectIds"`
			}
			if err := json.Unmarshal(raw, &entry); err != nil {
				continue
			}
			for _, pid := range entry.ProjectIDs {
				if pid == projectID {
					found = true
					if entry.IssueTypeScheme.ID == testSchemeID {
						return fmt.Errorf("project %s still has test scheme %s after destroy", projectID, testSchemeID)
					}
				}
			}
		}
		if !found {
			return fmt.Errorf("project %s has no issue type scheme association after destroy (expected revert to default)", projectID)
		}
	}
	return nil
}
