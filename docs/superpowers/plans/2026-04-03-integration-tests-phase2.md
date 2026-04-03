# Integration Tests Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **IMPORTANT:** Subagents MUST use the **Sonnet** model, not Opus. After each task commit, run Codex review (`git diff HEAD~1 | codex exec "..."`) before proceeding to the next task.

**Goal:** Add Phase 2 integration tests for the scheme chain: issue type scheme, workflow, status, and workflow scheme (including project associations).

**Architecture:** Integration tests live as `*_integration_test.go` files in `internal/jira/`. They use the `TestIntegration` prefix, are gated on `TF_ACC=1`, and hit the real Jira API. Sweepers clean up leaked `tf-acc-test-*` resources. Phase 1 infrastructure (shared helpers, CI workflow, TestMain) is already in place.

**Tech Stack:** Go 1.25, terraform-plugin-framework v1.19.0, terraform-plugin-testing v1.15.0

**Working directory:** `/Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian`

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `internal/jira/status_resource_integration_test.go` | Integration tests + sweeper for `jira_status` |
| **Create:** `internal/jira/workflow_resource_integration_test.go` | Integration tests + sweeper for `jira_workflow` |
| **Create:** `internal/jira/issue_type_scheme_resource_integration_test.go` | Integration tests + sweepers for `jira_issue_type_scheme` + `jira_project_issue_type_scheme` |
| **Create:** `internal/jira/workflow_scheme_resource_integration_test.go` | Integration tests + sweepers for `jira_workflow_scheme` + `jira_project_workflow_scheme` |

**Dependency order matters:** Statuses must exist before workflows can reference them. Workflows must exist before workflow schemes can reference them. Tests and sweepers are ordered accordingly.

---

### Task 1: Status Integration Tests + Sweeper

**Files:**
- Create: `internal/jira/status_resource_integration_test.go`

**Context:** The `jira_status` resource manages Jira Cloud statuses. Key API quirks:
- Create wraps the body in `{"statuses": [...]}` and returns an array
- Delete uses query param `?id=` (NOT a path segment)
- Read uses manual pagination via `/rest/api/3/statuses/search?id={id}`
- `status_category` is required and must be `"TODO"`, `"IN_PROGRESS"`, or `"DONE"` — it has `RequiresReplace`

Status must be created before workflows (which reference status IDs), so this task comes first.

- [ ] **Step 1: Read the status resource to understand schema and API**

Read: `internal/jira/status_resource.go`

Key attributes:
- `id` — computed string
- `name` — required string
- `description` — optional, computed, default `""`
- `status_category` — required, one of `TODO`/`IN_PROGRESS`/`DONE`, ForceNew

- [ ] **Step 2: Create the status integration test file**

```go
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
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_status", &resource.Sweeper{
		Name:         "atlassian_jira_status",
		Dependencies: []string{"atlassian_jira_workflow"},
		F:            sweepStatuses,
	})
}

func sweepStatuses(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// Statuses use paginated search endpoint.
	allValues, err := client.GetAllPages(ctx, "/rest/api/3/statuses/search")
	if err != nil {
		return fmt.Errorf("listing statuses for sweep: %w", err)
	}

	for _, raw := range allValues {
		var status struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &status); err != nil {
			continue
		}
		if !strings.HasPrefix(status.Name, "tf-acc-test-") {
			continue
		}

		// Status delete uses query param, NOT path segment.
		delPath := fmt.Sprintf("/rest/api/3/statuses?id=%s", atlassian.QueryEscape(status.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete status %q (%s): %s\n", status.Name, status.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationStatusResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "Integration test status (run %s)"
  status_category = "TODO"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "status_category", "TODO"),
					resource.TestCheckResourceAttrSet("atlassian_jira_status.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_status.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationStatusResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "original"
  status_category = "TODO"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", rName),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  description     = "updated"
  status_category = "TODO"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_status.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationStatusResource_disappears(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  status_category = "TODO"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testDeleteStatusOutOfBand("atlassian_jira_status.test"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegrationStatusDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckStatusDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_status" "test" {
  name            = %q
  status_category = "TODO"
}

data "atlassian_jira_status" "test" {
  name = atlassian_jira_status.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_status.test", "id"),
					resource.TestCheckResourceAttr("data.atlassian_jira_status.test", "status_category", "TODO"),
				),
			},
		},
	})
}

func testCheckStatusDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_status" {
			continue
		}
		// Status read uses search endpoint with ?id= param and manual pagination.
		// A simple GET to the search with the ID will return empty values if deleted.
		var result struct {
			Values []struct {
				ID string `json:"id"`
			} `json:"values"`
		}
		searchPath := fmt.Sprintf("/rest/api/3/statuses/search?id=%s&maxResults=1",
			atlassian.QueryEscape(rs.Primary.ID))
		if err := client.Get(ctx, searchPath, &result); err != nil {
			return fmt.Errorf("error checking status %s destruction: %w", rs.Primary.ID, err)
		}
		if len(result.Values) > 0 {
			return fmt.Errorf("status %s still exists", rs.Primary.ID)
		}
	}
	return nil
}

func testDeleteStatusOutOfBand(resourceAddr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceAddr)
		}
		client, err := testutil.SweepClient()
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}
		ctx := context.Background()
		// Status delete uses query param, NOT path segment.
		delPath := fmt.Sprintf("/rest/api/3/statuses?id=%s", atlassian.QueryEscape(rs.Primary.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			return fmt.Errorf("deleting status out-of-band: %w", delErr)
		}
		return nil
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 4: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationStatus' -v -count=1`
Expected: All tests show `--- SKIP`

- [ ] **Step 5: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jira/status_resource_integration_test.go
git commit -m "test: add status integration tests and sweeper"
```

---

### Task 2: Workflow Integration Tests + Sweeper

**Files:**
- Create: `internal/jira/workflow_resource_integration_test.go`

**Context:** The `jira_workflow` resource manages Jira Cloud workflows. Key API quirks:
- ALL fields have `RequiresReplace` — no in-place updates
- ID is a UUID nested under `id.entityId` in API responses
- Create uses `POST /rest/api/3/workflow/create` (not just `/workflow`)
- Delete uses `?workflowId=` query param
- `statuses` is a list of status IDs (status UUIDs/references)
- Search endpoint: `GET /rest/api/3/workflow/search` with optional `workflowName` param

The workflow test creates a status first (as a dependency) and references its ID in the workflow's `statuses` list.

- [ ] **Step 1: Read the workflow resource to understand schema and API**

Read: `internal/jira/workflow_resource.go`

Key attributes:
- `id` — computed string (UUID from `id.entityId`)
- `name` — required, ForceNew
- `description` — optional, computed, default `""`, ForceNew
- `statuses` — required list of strings (status references), ForceNew

- [ ] **Step 2: Create the workflow integration test file**

```go
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
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_workflow", &resource.Sweeper{
		Name:         "atlassian_jira_workflow",
		Dependencies: []string{"atlassian_jira_workflow_scheme"},
		F:            sweepWorkflows,
	})
}

func sweepWorkflows(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// Workflow search is paginated with "values" key.
	allValues, err := client.GetAllPages(ctx, "/rest/api/3/workflow/search")
	if err != nil {
		return fmt.Errorf("listing workflows for sweep: %w", err)
	}

	for _, raw := range allValues {
		var wf struct {
			ID struct {
				EntityID string `json:"entityId"`
			} `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &wf); err != nil {
			continue
		}
		if !strings.HasPrefix(wf.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/workflow?workflowId=%s", atlassian.QueryEscape(wf.ID.EntityID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete workflow %q (%s): %s\n", wf.Name, wf.ID.EntityID, delErr)
		}
	}

	return nil
}

func TestIntegrationWorkflowResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationWorkflowConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_workflow.test", "id"),
				),
			},
			{
				ResourceName:            "atlassian_jira_workflow.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"statuses"},
			},
		},
	})
}

func TestIntegrationWorkflowDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationWorkflowConfig(rName) + fmt.Sprintf(`
data "atlassian_jira_workflow" "test" {
  name = atlassian_jira_workflow.test.name
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_workflow.test", "id"),
				),
			},
		},
	})
}

// testIntegrationWorkflowConfig creates a status dependency and a workflow that references it.
func testIntegrationWorkflowConfig(name string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_status" "wf_dep" {
  name            = "%s-status"
  description     = "Workflow dep status (run %s)"
  status_category = "TODO"
}

resource "atlassian_jira_workflow" "test" {
  name        = %q
  description = "Integration test workflow (run %s)"
  statuses    = [atlassian_jira_status.wf_dep.id]
}
`, name, testutil.RunID(), name, testutil.RunID())
}

func testCheckWorkflowDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_workflow" {
			continue
		}
		// Search for workflow by name — if not found, it's destroyed.
		var result struct {
			Values []struct {
				ID struct {
					EntityID string `json:"entityId"`
				} `json:"id"`
			} `json:"values"`
		}
		searchPath := fmt.Sprintf("/rest/api/3/workflow/search?workflowName=%s",
			atlassian.QueryEscape(rs.Primary.Attributes["name"]))
		if err := client.Get(ctx, searchPath, &result); err != nil {
			return fmt.Errorf("error checking workflow %s destruction: %w", rs.Primary.ID, err)
		}
		for _, wf := range result.Values {
			if wf.ID.EntityID == rs.Primary.ID {
				return fmt.Errorf("workflow %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
```

**Note on `ImportStateVerifyIgnore: []string{"statuses"}`:** The design spec (line 266) explicitly lists `statuses` for `jira_workflow` as an ignored field because the server may reorder status references during import.

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 4: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationWorkflow' -v -count=1`
Expected: Both tests show `--- SKIP`

- [ ] **Step 5: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jira/workflow_resource_integration_test.go
git commit -m "test: add workflow integration tests and sweeper"
```

---

### Task 3: Issue Type Scheme Integration Tests + Sweepers

**Files:**
- Create: `internal/jira/issue_type_scheme_resource_integration_test.go`

**Context:** The `jira_issue_type_scheme` resource manages issue type schemes. It also covers the `jira_project_issue_type_scheme` association. Key API quirks:
- Create returns `{"issueTypeSchemeId": "..."}` (not `id`)
- Read uses paginated list at `/rest/api/3/issuetypescheme` — find by ID
- Mappings are at a separate endpoint: `/rest/api/3/issuetypescheme/mapping?issueTypeSchemeId={id}`
- `issue_type_ids` is a required list — order matters
- `project_issue_type_scheme` uses `PUT /rest/api/3/issuetypescheme/project` to assign, delete reverts to default scheme ID `10000`
- `project_issue_type_scheme` import is by project ID (custom logic)

- [ ] **Step 1: Read the resources to understand schemas**

Read:
- `internal/jira/issue_type_scheme_resource.go`
- `internal/jira/project_issue_type_scheme_resource.go`

- [ ] **Step 2: Create the issue type scheme integration test file**

```go
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
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func init() {
	resource.AddTestSweepers("atlassian_jira_project_issue_type_scheme", &resource.Sweeper{
		Name: "atlassian_jira_project_issue_type_scheme",
		F:    sweepProjectIssueTypeSchemes,
	})
	resource.AddTestSweepers("atlassian_jira_issue_type_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_issue_type_scheme",
		Dependencies: []string{"atlassian_jira_project_issue_type_scheme"},
		F:            sweepIssueTypeSchemes,
	})
}

func sweepProjectIssueTypeSchemes(_ string) error {
	// Project issue type scheme associations are cleaned up when projects are deleted
	// by the atlassian_jira_project sweeper, or when schemes are deleted.
	return nil
}

func sweepIssueTypeSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/issuetypescheme")
	if err != nil {
		return fmt.Errorf("listing issue type schemes for sweep: %w", err)
	}

	for _, raw := range allValues {
		var scheme struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &scheme); err != nil {
			continue
		}
		if !strings.HasPrefix(scheme.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s", atlassian.PathEscape(scheme.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete issue type scheme %q (%s): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationIssueTypeSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_issue_type_scheme.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_issue_type_scheme.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationIssueTypeSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeSchemeConfig(rName, "original"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", rName),
				),
			},
			{
				Config: testIntegrationIssueTypeSchemeConfig(rNameUpdated, "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type_scheme.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationIssueTypeSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationIssueTypeSchemeConfig(rName, "ds-test") + `
data "atlassian_jira_issue_type_scheme" "test" {
  name = atlassian_jira_issue_type_scheme.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_issue_type_scheme.test", "id"),
				),
			},
		},
	})
}

// testIntegrationIssueTypeSchemeConfig creates an issue type and an issue type scheme
// that references it. The "10000" is the built-in "Task" issue type present in all Jira instances.
func testIntegrationIssueTypeSchemeConfig(name, description string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_issue_type" "its_dep" {
  name = "%s-issuetype"
  type = "standard"
}

resource "atlassian_jira_issue_type_scheme" "test" {
  name           = %q
  description    = %q
  issue_type_ids = [atlassian_jira_issue_type.its_dep.id]
}
`, name, name, description)
}

func testCheckIssueTypeSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_issue_type_scheme" {
			continue
		}
		// List all schemes and check if the ID still exists.
		allValues, err := client.GetAllPages(ctx, "/rest/api/3/issuetypescheme")
		if err != nil {
			return fmt.Errorf("error listing issue type schemes: %w", err)
		}
		for _, raw := range allValues {
			var scheme struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &scheme); err != nil {
				continue
			}
			if scheme.ID == rs.Primary.ID {
				return fmt.Errorf("issue type scheme %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 4: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationIssueTypeScheme' -v -count=1`
Expected: All tests show `--- SKIP`

- [ ] **Step 5: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jira/issue_type_scheme_resource_integration_test.go
git commit -m "test: add issue type scheme integration tests and sweepers"
```

---

### Task 4: Workflow Scheme Integration Tests + Sweepers

**Files:**
- Create: `internal/jira/workflow_scheme_resource_integration_test.go`

**Context:** The `jira_workflow_scheme` resource manages workflow schemes. It also covers the `jira_project_workflow_scheme` association. Key API quirks:
- ID is numeric `int64` in API, stored as string
- Read is a direct GET (not paginated list)
- `default_workflow` and `issue_type_mappings` are optional+computed with `UseStateForUnknown`
- `issue_type_mappings` is `map[string]string` (issue type ID → workflow name)
- `project_workflow_scheme` delete is a no-op (Jira doesn't support removing the association)
- `project_workflow_scheme` import is by project ID

The test creates a status → workflow → workflow scheme chain to exercise the full dependency path.

- [ ] **Step 1: Read the resources to understand schemas**

Read:
- `internal/jira/workflow_scheme_resource.go`
- `internal/jira/project_workflow_scheme_resource.go`

- [ ] **Step 2: Create the workflow scheme integration test file**

```go
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
	resource.AddTestSweepers("atlassian_jira_project_workflow_scheme", &resource.Sweeper{
		Name: "atlassian_jira_project_workflow_scheme",
		F:    sweepProjectWorkflowSchemes,
	})
	resource.AddTestSweepers("atlassian_jira_workflow_scheme", &resource.Sweeper{
		Name:         "atlassian_jira_workflow_scheme",
		Dependencies: []string{"atlassian_jira_project_workflow_scheme"},
		F:            sweepWorkflowSchemes,
	})
}

func sweepProjectWorkflowSchemes(_ string) error {
	// Project workflow scheme delete is a no-op in the provider.
	// Associations are cleaned up when projects are deleted.
	return nil
}

func sweepWorkflowSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// Workflow scheme list is paginated.
	allValues, err := client.GetAllPages(ctx, "/rest/api/3/workflowscheme")
	if err != nil {
		return fmt.Errorf("listing workflow schemes for sweep: %w", err)
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

		delPath := fmt.Sprintf("/rest/api/3/workflowscheme/%d", scheme.ID)
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete workflow scheme %q (%d): %s\n", scheme.Name, scheme.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationWorkflowSchemeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "Integration test workflow scheme (run %s)"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_workflow_scheme.test", "id"),
				),
			},
			{
				ResourceName:            "atlassian_jira_workflow_scheme.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"issue_type_mappings"},
			},
		},
	})
}

func TestIntegrationWorkflowSchemeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "original"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", rName),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name        = %q
  description = "updated"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_workflow_scheme.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationWorkflowSchemeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckWorkflowSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_workflow_scheme" "test" {
  name = %q
}

data "atlassian_jira_workflow_scheme" "test" {
  name = atlassian_jira_workflow_scheme.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_workflow_scheme.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_workflow_scheme.test", "id"),
				),
			},
		},
	})
}

func testCheckWorkflowSchemeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_workflow_scheme" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/workflowscheme/%s", atlassian.PathEscape(rs.Primary.ID)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking workflow scheme %s destruction: %w", rs.Primary.ID, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("workflow scheme %s still exists (status %d)", rs.Primary.ID, statusCode)
		}
	}
	return nil
}
```

**Note on `ImportStateVerifyIgnore: []string{"issue_type_mappings"}`:** The design spec (line 267) lists this for `jira_workflow_scheme` because of empty map vs null distinction on import.

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors. If there's a missing `json` import for the sweeper, add it.

- [ ] **Step 4: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationWorkflowScheme' -v -count=1`
Expected: All tests show `--- SKIP`

- [ ] **Step 5: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jira/workflow_scheme_resource_integration_test.go
git commit -m "test: add workflow scheme integration tests and sweepers"
```

---

### Task 5: Final Verification and Push

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./... -v -count=1 -timeout 5m`
Expected: All existing tests PASS, all new `TestIntegration*` tests SKIP

- [ ] **Step 2: Run full linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./...`
Expected: No errors

- [ ] **Step 3: Run build**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./...`
Expected: No errors

- [ ] **Step 4: Push branch and create PR**

```bash
git push origin feat/integration-tests-phase2
```

Then create the PR:

```bash
gh pr create \
  --title "test: add integration tests Phase 2 (scheme chain)" \
  --base main \
  --head feat/integration-tests-phase2 \
  --body "$(cat <<'EOF'
## Summary

- Adds Phase 2 integration tests for the scheme chain resources
- Covers: status, workflow, issue type scheme, workflow scheme

## Changes

- `internal/jira/status_resource_integration_test.go` — 4 tests + sweeper
- `internal/jira/workflow_resource_integration_test.go` — 2 tests + sweeper
- `internal/jira/issue_type_scheme_resource_integration_test.go` — 3 tests + sweepers
- `internal/jira/workflow_scheme_resource_integration_test.go` — 3 tests + sweepers

## Test plan

- [ ] All mock tests still pass (`go test ./... -count=1`)
- [ ] All integration tests skip without `TF_ACC=1`
- [ ] Linter passes
- [ ] Integration tests pass on merge to main
EOF
)"
```
