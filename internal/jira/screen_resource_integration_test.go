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
	resource.AddTestSweepers("atlassian_jira_screen", &resource.Sweeper{
		Name:         "atlassian_jira_screen",
		Dependencies: []string{"atlassian_jira_screen_scheme"},
		F:            sweepScreens,
	})
}

func sweepScreens(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	startAt := 0
	maxResults := 100

	for {
		apiPath := fmt.Sprintf("/rest/api/3/screens?maxResults=%d&startAt=%d", maxResults, startAt)

		var page struct {
			Values []json.RawMessage `json:"values"`
			IsLast bool              `json:"isLast"`
		}

		if err := client.Get(ctx, apiPath, &page); err != nil {
			return fmt.Errorf("listing screens for sweep: %w", err)
		}

		for _, raw := range page.Values {
			var s struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &s); err != nil {
				fmt.Printf("[WARN] Failed to unmarshal screen: %s\n", err)
				continue
			}

			if !strings.HasPrefix(s.Name, "tf-acc-test-") {
				continue
			}

			delPath := fmt.Sprintf("/rest/api/3/screens/%d", s.ID)
			_, delErr := client.DeleteWithStatus(ctx, delPath)
			if delErr != nil {
				fmt.Printf("[WARN] Failed to delete screen %q (%d): %s\n", s.Name, s.ID, delErr)
			}
		}

		if page.IsLast || len(page.Values) == 0 {
			break
		}

		startAt += len(page.Values)
	}

	return nil
}

func TestIntegrationScreenResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenConfig(rName, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_screen.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationScreenResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenConfig(rName, "initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "description", "initial description"),
				),
			},
			{
				Config: testIntegrationScreenConfig(rNameUpdated, "updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_screen.test", "description", "updated description"),
				),
			},
		},
	})
}

func TestIntegrationScreenTabResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenTabConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_screen.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_tab.test", "name", rName+"-tab"),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_tab.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_screen_tab.test",
				ImportState:       true,
				ImportStateIdFunc: testImportScreenTabID("atlassian_jira_screen_tab.test"),
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationScreenTabFieldResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckScreenDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationScreenTabFieldConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("atlassian_jira_screen.test", "id"),
					resource.TestCheckResourceAttrSet("atlassian_jira_screen_tab.test", "id"),
					resource.TestCheckResourceAttr("atlassian_jira_screen_tab_field.test", "field_id", "summary"),
				),
			},
			{
				ResourceName:      "atlassian_jira_screen_tab_field.test",
				ImportState:       true,
				ImportStateIdFunc: testImportScreenTabFieldID("atlassian_jira_screen_tab_field.test"),
				ImportStateVerify: true,
			},
		},
	})
}

// testCheckScreenDestroyed verifies that all atlassian_jira_screen resources in state
// have been deleted by paginating through the screens list.
func testCheckScreenDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_screen" {
			continue
		}
		screenID := rs.Primary.ID

		found, err := screenExistsByID(ctx, client, screenID)
		if err != nil {
			return fmt.Errorf("error checking screen %s destruction: %w", screenID, err)
		}
		if found {
			return fmt.Errorf("screen %s still exists", screenID)
		}
	}
	return nil
}

// screenExistsByID paginates GET /rest/api/3/screens to check whether the given ID exists.
func screenExistsByID(ctx context.Context, client *atlassian.Client, id string) (bool, error) {
	startAt := 0
	maxResults := 100

	for {
		apiPath := fmt.Sprintf("/rest/api/3/screens?maxResults=%d&startAt=%d", maxResults, startAt)

		var page struct {
			Values  []json.RawMessage `json:"values"`
			IsLast  bool              `json:"isLast"`
		}

		statusCode, err := client.GetWithStatus(ctx, apiPath, &page)
		if err != nil {
			return false, err
		}
		if statusCode == http.StatusNotFound {
			return false, nil
		}

		for _, raw := range page.Values {
			var s struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &s); err != nil {
				return false, fmt.Errorf("unmarshaling screen: %w", err)
			}
			if fmt.Sprintf("%d", s.ID) == id {
				return true, nil
			}
		}

		if page.IsLast || len(page.Values) == 0 {
			break
		}

		startAt += len(page.Values)
	}

	return false, nil
}

// testImportScreenTabID returns an ImportStateIdFunc that builds "{screen_id}/{tab_id}".
func testImportScreenTabID(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceAddr)
		}
		screenID := rs.Primary.Attributes["screen_id"]
		tabID := rs.Primary.ID
		if screenID == "" || tabID == "" {
			return "", fmt.Errorf("screen_id or id is empty for resource %s", resourceAddr)
		}
		return fmt.Sprintf("%s/%s", screenID, tabID), nil
	}
}

// testImportScreenTabFieldID returns an ImportStateIdFunc that builds "{screen_id}/{tab_id}/{field_id}".
func testImportScreenTabFieldID(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceAddr)
		}
		screenID := rs.Primary.Attributes["screen_id"]
		tabID := rs.Primary.Attributes["tab_id"]
		fieldID := rs.Primary.Attributes["field_id"]
		if screenID == "" || tabID == "" || fieldID == "" {
			return "", fmt.Errorf("screen_id, tab_id, or field_id is empty for resource %s", resourceAddr)
		}
		return fmt.Sprintf("%s/%s/%s", screenID, tabID, fieldID), nil
	}
}

func testIntegrationScreenConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_screen" "test" {
  name        = %q
  description = %q
}
`, name, description)
}

func testIntegrationScreenTabConfig(name string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_screen" "test" {
  name        = %q
  description = "Integration test screen (run %s)"
}

resource "atlassian_jira_screen_tab" "test" {
  screen_id = atlassian_jira_screen.test.id
  name      = %q
}
`, name, testutil.RunID(), name+"-tab")
}

func testIntegrationScreenTabFieldConfig(name string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_screen" "test" {
  name        = %q
  description = "Integration test screen (run %s)"
}

resource "atlassian_jira_screen_tab" "test" {
  screen_id = atlassian_jira_screen.test.id
  name      = %q
}

resource "atlassian_jira_screen_tab_field" "test" {
  screen_id = atlassian_jira_screen.test.id
  tab_id    = atlassian_jira_screen_tab.test.id
  field_id  = "summary"
}
`, name, testutil.RunID(), name+"-tab")
}
