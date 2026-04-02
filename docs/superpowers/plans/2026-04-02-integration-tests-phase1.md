# Integration Tests Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **IMPORTANT:** Subagents MUST use the **Sonnet** model, not Opus. After each task commit, run the `/codex-review` skill (or pipe the diff to `codex exec`) before proceeding to the next task.

**Goal:** Add real-API integration test infrastructure (CI workflow, shared helpers, sweepers) and Phase 1 integration tests for group, project, issue type, and permission scheme resources.

**Architecture:** Integration tests live as `*_integration_test.go` files alongside existing mock tests in `internal/jira/`. They use the `TestIntegration` prefix, are gated on `TF_ACC=1`, and run against the real Jira instance. A GitHub Actions workflow runs them on merge to `main` within a protected `integration` environment. Sweeper functions clean up leaked `tf-acc-test-*` resources.

**Tech Stack:** Go 1.25, terraform-plugin-framework v1.19.0, terraform-plugin-testing v1.15.0, GitHub Actions

**Working directory:** `/Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian`

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `internal/testutil/integration.go` | Shared helpers: `SkipIfNoAcc`, `SweepClient` (with tenant allowlist), `RunID` |
| **Create:** `.github/workflows/integration-test.yml` | CI workflow: runs on merge to main, protected environment, sweeper step |
| **Modify:** `Makefile` | Add `testintegration` and `sweep` targets |
| **Create:** `internal/jira/test_main_test.go` | Package-level `TestMain` calling `resource.TestMain(m)` — required for `-sweep` flag to work |
| **Create:** `internal/jira/group_resource_integration_test.go` | Integration tests + sweeper for `jira_group` |
| **Create:** `internal/jira/project_resource_integration_test.go` | Integration tests + sweeper for `jira_project` |
| **Create:** `internal/jira/issue_type_resource_integration_test.go` | Integration tests + sweeper for `jira_issue_type` |
| **Create:** `internal/jira/permission_scheme_resource_integration_test.go` | Integration tests + sweepers for `jira_permission_scheme`, grant, project assoc |

---

### Task 1: Shared Test Infrastructure

**Files:**
- Create: `internal/testutil/integration.go`

- [ ] **Step 1: Create integration.go with SkipIfNoAcc, SweepClient, RunID**

```go
package testutil

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

// AllowedSweepHosts is the set of Jira instances that sweepers are allowed to operate on.
// This prevents accidental sweeps against production instances.
var AllowedSweepHosts = map[string]bool{
	"lbajsarowicz.atlassian.net": true,
}

// SkipIfNoAcc skips the test unless TF_ACC=1 is set.
func SkipIfNoAcc(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping integration test")
	}
}

// SweepClient returns a real atlassian.Client for sweeper cleanup.
// Validates that the target host is in AllowedSweepHosts.
// Outside CI (GITHUB_ACTIONS != "true"), requires ATLASSIAN_SWEEP_CONFIRM=<hostname>.
func SweepClient() (*atlassian.Client, error) {
	atlassianURL := os.Getenv("ATLASSIAN_URL")
	if atlassianURL == "" {
		return nil, fmt.Errorf("ATLASSIAN_URL is not set")
	}

	parsed, err := url.Parse(atlassianURL)
	if err != nil {
		return nil, fmt.Errorf("parsing ATLASSIAN_URL: %w", err)
	}

	host := parsed.Hostname()
	if !AllowedSweepHosts[host] {
		return nil, fmt.Errorf("sweep aborted: host %q is not in the allowlist", host)
	}

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		confirm := os.Getenv("ATLASSIAN_SWEEP_CONFIRM")
		if confirm != host {
			return nil, fmt.Errorf(
				"sweep aborted: set ATLASSIAN_SWEEP_CONFIRM=%s to confirm local sweep", host)
		}
	}

	client, err := atlassian.NewClient(atlassian.ClientConfig{})
	if err != nil {
		return nil, fmt.Errorf("creating sweep client: %w", err)
	}

	return client, nil
}

// RunID returns a truncated GitHub run ID for resource tagging, or "local" for local runs.
func RunID() string {
	if id := os.Getenv("GITHUB_RUN_ID"); id != "" {
		if len(id) > 8 {
			return id[:8]
		}
		return id
	}
	return "local"
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/testutil/...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/testutil/integration.go
git commit -m "feat: add shared integration test helpers (SkipIfNoAcc, SweepClient, RunID)"
```

---

### Task 2: CI Workflow and Makefile

**Files:**
- Create: `.github/workflows/integration-test.yml`
- Modify: `Makefile`

- [ ] **Step 1: Create the integration test CI workflow**

```yaml
name: Integration Tests

on:
  push:
    branches: [main]
  workflow_dispatch: {}

permissions:
  contents: read

concurrency:
  group: integration-tests
  cancel-in-progress: false

jobs:
  integration:
    name: Integration
    runs-on: ubuntu-latest
    timeout-minutes: 45
    environment: integration
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Run integration tests
        env:
          TF_ACC: "1"
          ATLASSIAN_URL: ${{ secrets.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ secrets.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: go test ./internal/jira/... -v -count=1 -timeout 30m -run '^TestIntegration'

      - name: Sweep stale resources
        if: always()
        env:
          ATLASSIAN_URL: ${{ secrets.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ secrets.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: go test ./internal/jira/... -v -sweep=all -timeout 10m
```

- [ ] **Step 2: Add Makefile targets**

Add the following to the end of the `Makefile` (before the final blank line):

```makefile
testintegration:
	TF_ACC=1 go test ./internal/jira/... -v -count=1 -timeout 30m -run '^TestIntegration'

sweep:
	go test ./internal/jira/... -v -sweep=all -timeout 10m
```

- [ ] **Step 3: Verify the YAML is valid**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && python3 -c "import yaml; yaml.safe_load(open('.github/workflows/integration-test.yml'))"`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/integration-test.yml Makefile
git commit -m "ci: add integration test workflow and Makefile targets"
```

---

### Task 3: Preflight Test

**Files:**
- Create: `internal/jira/integration_preflight_test.go`

- [ ] **Step 1: Create the preflight test**

```go
package jira_test

import (
	"context"
	"testing"

	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

// TestIntegration_preflight verifies API access and admin permissions before the suite runs.
// If this test fails, all other integration tests will also fail — check credentials.
func TestIntegration_preflight(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	client, err := testutil.SweepClient()
	if err != nil {
		t.Fatalf("preflight: cannot create client: %s", err)
	}

	ctx := context.Background()

	// Verify we can list projects (requires project-level read access).
	var result interface{}
	statusCode, err := client.GetWithStatus(ctx, "/rest/api/3/project/search?maxResults=1", &result)
	if err != nil {
		t.Fatalf("preflight: API access failed (status %d): %s", statusCode, err)
	}

	// Verify we can list groups (requires admin-level access).
	var groupResult interface{}
	statusCode, err = client.GetWithStatus(ctx, "/rest/api/3/group/bulk?maxResults=1", &groupResult)
	if err != nil {
		t.Fatalf("preflight: admin access failed (status %d) — check that the API token belongs to a Jira admin: %s", statusCode, err)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 3: Verify the test is skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegration_preflight$' -v -count=1`
Expected: `--- SKIP: TestIntegration_preflight`

- [ ] **Step 4: Commit**

```bash
git add internal/jira/integration_preflight_test.go
git commit -m "test: add integration preflight test for API access verification"
```

---

### Task 3.5: TestMain for Sweeper Support

**Files:**
- Create: `internal/jira/test_main_test.go`

The `-sweep` flag in `terraform-plugin-testing` only works when the package defines a `TestMain` that calls `resource.TestMain(m)`. Without this, `go test -sweep=all` is silently a no-op.

- [ ] **Step 1: Create test_main_test.go**

```go
package jira_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestMain(m *testing.M) {
	resource.TestMain(m)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 3: Verify existing mock tests still pass**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -v -count=1 -timeout 5m 2>&1 | tail -5`
Expected: All tests PASS (TestMain with resource.TestMain delegates to m.Run() when -sweep is not set)

- [ ] **Step 4: Commit**

```bash
git add internal/jira/test_main_test.go
git commit -m "test: add TestMain for sweeper support"
```

---

### Task 4: Group Integration Tests + Sweeper

**Files:**
- Create: `internal/jira/group_resource_integration_test.go`

- [ ] **Step 1: Create the group integration test file**

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
	resource.AddTestSweepers("atlassian_jira_group", &resource.Sweeper{
		Name: "atlassian_jira_group",
		F:    sweepGroups,
	})
}

func sweepGroups(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// List all groups using the bulk endpoint.
	allValues, err := client.GetAllPages(ctx, "/rest/api/3/group/bulk")
	if err != nil {
		return fmt.Errorf("listing groups for sweep: %w", err)
	}

	for _, raw := range allValues {
		var group struct {
			GroupID string `json:"groupId"`
			Name    string `json:"name"`
		}
		if err := json.Unmarshal(raw, &group); err != nil {
			continue
		}
		if !strings.HasPrefix(group.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(group.GroupID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete group %q (%s): %s\n", group.Name, group.GroupID, delErr)
		}
	}

	return nil
}

func TestIntegrationGroupResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckGroupDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_group.test", "group_id"),
				),
			},
			{
				ResourceName:                         "atlassian_jira_group.test",
				ImportState:                          true,
				ImportStateId:                        rName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group_id",
			},
		},
	})
}

func TestIntegrationGroupResource_disappears(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckGroupDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Delete the group out-of-band to simulate external deletion.
					testDeleteGroupOutOfBand("atlassian_jira_group.test"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegrationGroupDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckGroupDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_group" "test" { name = %q }

data "atlassian_jira_group" "test" {
  name = atlassian_jira_group.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_group.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_group.test", "group_id"),
				),
			},
		},
	})
}

func testCheckGroupDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_group" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(rs.Primary.Attributes["group_id"])),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking group %s destruction: %w", rs.Primary.Attributes["group_id"], err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("group %s still exists (status %d)", rs.Primary.Attributes["group_id"], statusCode)
		}
	}
	return nil
}

func testDeleteGroupOutOfBand(resourceAddr string) resource.TestCheckFunc {
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
		groupID := rs.Primary.Attributes["group_id"]
		delPath := fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(groupID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			return fmt.Errorf("deleting group out-of-band: %w", delErr)
		}
		return nil
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 3: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationGroup' -v -count=1`
Expected: All three tests show `--- SKIP`

- [ ] **Step 4: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/jira/group_resource_integration_test.go
git commit -m "test: add group integration tests and sweeper"
```

---

### Task 5: Project Integration Tests + Sweeper

**Files:**
- Create: `internal/jira/project_resource_integration_test.go`

Note: Read `internal/jira/project_resource.go` first to understand the schema (key, name, project_type_key, lead_account_id, description).

- [ ] **Step 1: Read the project resource to understand schema and API**

Read: `internal/jira/project_resource.go`

Identify:
- The Terraform attribute names and which are required/optional/computed
- The API endpoints used (POST `/rest/api/3/project`, GET `/rest/api/3/project/{key}`, PUT, DELETE)
- The import pattern (import by key)
- Which fields have ForceNew

- [ ] **Step 2: Create the project integration test file**

Note: The `lead_account_id` field is required for creating Jira projects. The test uses the `myself` API endpoint to look up the authenticated user's account ID at test time.

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
	resource.AddTestSweepers("atlassian_jira_project", &resource.Sweeper{
		Name:         "atlassian_jira_project",
		Dependencies: []string{"atlassian_jira_group"},
		F:            sweepProjects,
	})
}

func sweepProjects(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	allValues, err := client.GetAllPages(ctx, "/rest/api/3/project/search")
	if err != nil {
		return fmt.Errorf("listing projects for sweep: %w", err)
	}

	for _, raw := range allValues {
		var project struct {
			ID   string `json:"id"`
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &project); err != nil {
			continue
		}
		if !strings.HasPrefix(project.Name, "tf-acc-test-") && !strings.HasPrefix(project.Key, "TFACC") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(project.Key))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete project %q (%s): %s\n", project.Name, project.Key, delErr)
		}
	}

	return nil
}

func TestIntegrationProjectResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rSuffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rKey := "TFACC" + rSuffix
	rName := acctest.RandomWithPrefix("tf-acc-test")
	leadAccountID := testIntegrationProjectLeadAccountID(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectConfig(rKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "key", rKey),
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", rName),
					resource.TestCheckResourceAttrSet("atlassian_jira_project.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_project.test",
				ImportState:       true,
				ImportStateId:     rKey,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationProjectResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rSuffix := acctest.RandStringFromCharSet(4, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rKey := "TFACC" + rSuffix
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-updated"
	leadAccountID := testIntegrationProjectLeadAccountID(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckProjectDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testIntegrationProjectConfig(rKey, rName, leadAccountID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", rName),
				),
			},
			{
				Config: testIntegrationProjectConfig(rKey, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project.test", "name", rNameUpdated),
				),
			},
		},
	})
}

// testIntegrationProjectLeadAccountID fetches the authenticated user's account ID
// from the Jira "myself" endpoint. This is used as lead_account_id in project tests.
func testIntegrationProjectLeadAccountID(t *testing.T) string {
	t.Helper()
	client, err := testutil.SweepClient()
	if err != nil {
		t.Fatalf("creating client to look up lead account ID: %s", err)
	}
	var myself struct {
		AccountID string `json:"accountId"`
	}
	if err := client.Get(context.Background(), "/rest/api/3/myself", &myself); err != nil {
		t.Fatalf("fetching current user for lead_account_id: %s", err)
	}
	return myself.AccountID
}

func testIntegrationProjectConfig(key, name, leadAccountID string) string {
	return fmt.Sprintf(`
resource "atlassian_jira_project" "test" {
  key              = %q
  name             = %q
  project_type_key = "software"
  lead_account_id  = %q
  description      = "Integration test project (run %s)"
}
`, key, name, leadAccountID, testutil.RunID())
}

func testCheckProjectDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_project" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(rs.Primary.Attributes["key"])),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking project %s destruction: %w", rs.Primary.Attributes["key"], err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("project %s still exists (status %d)", rs.Primary.Attributes["key"], statusCode)
		}
	}
	return nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 4: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationProject' -v -count=1`
Expected: All tests show `--- SKIP`

- [ ] **Step 5: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jira/project_resource_integration_test.go
git commit -m "test: add project integration tests and sweeper"
```

---

### Task 6: Issue Type Integration Tests + Sweeper

**Files:**
- Create: `internal/jira/issue_type_resource_integration_test.go`

Note: Read `internal/jira/issue_type_resource.go` first. Issue types use: name, description, type (standard/subtask), hierarchy_level. The API is at `/rest/api/3/issuetype`. ID is a string in the API response.

- [ ] **Step 1: Read the issue type resource to understand schema and API**

Read: `internal/jira/issue_type_resource.go`

Identify:
- Required attributes: `name`, `type` (one of: `"standard"`, `"subtask"`)
- Optional attributes: `description`
- Computed attributes: `id`, `hierarchy_level`
- API: POST/GET/PUT/DELETE `/rest/api/3/issuetype` / `/rest/api/3/issuetype/{id}`
- Import: by ID

- [ ] **Step 2: Create the issue type integration test file**

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
	resource.AddTestSweepers("atlassian_jira_issue_type", &resource.Sweeper{
		Name: "atlassian_jira_issue_type",
		F:    sweepIssueTypes,
	})
}

func sweepIssueTypes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// GET /rest/api/3/issuetype returns a flat list (not paginated).
	var issueTypes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := client.Get(ctx, "/rest/api/3/issuetype", &issueTypes); err != nil {
		return fmt.Errorf("listing issue types for sweep: %w", err)
	}

	for _, it := range issueTypes {
		if !strings.HasPrefix(it.Name, "tf-acc-test-") {
			continue
		}

		delPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(it.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			fmt.Printf("[WARN] Failed to delete issue type %q (%s): %s\n", it.Name, it.ID, delErr)
		}
	}

	return nil
}

func TestIntegrationIssueTypeResource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name        = %q
  description = "Integration test issue type (run %s)"
  type        = "standard"
}
`, rName, testutil.RunID()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "type", "standard"),
					resource.TestCheckResourceAttrSet("atlassian_jira_issue_type.test", "id"),
				),
			},
			{
				ResourceName:      "atlassian_jira_issue_type.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegrationIssueTypeResource_update(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name        = %q
  description = "original"
  type        = "standard"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", rName),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "original"),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name        = %q
  description = "updated"
  type        = "standard"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_issue_type.test", "description", "updated"),
				),
			},
		},
	})
}

func TestIntegrationIssueTypeResource_disappears(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "standard"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testDeleteIssueTypeOutOfBand("atlassian_jira_issue_type.test"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegrationIssueTypeDataSource_basic(t *testing.T) {
	testutil.SkipIfNoAcc(t)
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckIssueTypeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_issue_type" "test" {
  name = %q
  type = "standard"
}

data "atlassian_jira_issue_type" "test" {
  name = atlassian_jira_issue_type.test.name
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_issue_type.test", "name", rName),
					resource.TestCheckResourceAttrSet("data.atlassian_jira_issue_type.test", "id"),
				),
			},
		},
	})
}

func testCheckIssueTypeDestroyed(s *terraform.State) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "atlassian_jira_issue_type" {
			continue
		}
		var result interface{}
		statusCode, err := client.GetWithStatus(ctx,
			fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(rs.Primary.ID)),
			&result,
		)
		if err != nil {
			return fmt.Errorf("error checking issue type %s destruction: %w", rs.Primary.ID, err)
		}
		if statusCode != http.StatusNotFound {
			return fmt.Errorf("issue type %s still exists (status %d)", rs.Primary.ID, statusCode)
		}
	}
	return nil
}

func testDeleteIssueTypeOutOfBand(resourceAddr string) resource.TestCheckFunc {
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
		delPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(rs.Primary.ID))
		_, delErr := client.DeleteWithStatus(ctx, delPath)
		if delErr != nil {
			return fmt.Errorf("deleting issue type out-of-band: %w", delErr)
		}
		return nil
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 4: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationIssueType' -v -count=1`
Expected: All tests show `--- SKIP`

- [ ] **Step 5: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jira/issue_type_resource_integration_test.go
git commit -m "test: add issue type integration tests and sweeper"
```

---

### Task 7: Permission Scheme Integration Tests + Sweepers

**Files:**
- Create: `internal/jira/permission_scheme_resource_integration_test.go`

Note: This file covers `jira_permission_scheme`, `jira_permission_scheme_grant`, and `jira_project_permission_scheme`. Read the corresponding resource files first.

- [ ] **Step 1: Read the permission scheme resources**

Read:
- `internal/jira/permission_scheme_resource.go`
- `internal/jira/permission_scheme_grant_resource.go`
- `internal/jira/project_permission_scheme_resource.go`

Identify:
- Permission scheme: name, description. API: `/rest/api/3/permissionscheme`. Note: list endpoint uses `permissionSchemes` key, NOT standard paginated `values`.
- Grant: scheme_id, holder_type, holder_parameter, permission. API: `/rest/api/3/permissionscheme/{schemeId}/permission`.
- Project assoc: `project_key`, `scheme_id`. Delete reassigns to default scheme. ForceNew on both fields.

- [ ] **Step 2: Create the permission scheme integration test file**

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
	// The project permission scheme resource's Delete reassigns the project to the
	// default scheme (ID 10000). To ensure custom test schemes can be deleted,
	// we reassign any test projects back to the default scheme here.
	// This is handled implicitly when the project itself is deleted by the project sweeper.
	return nil
}

func sweepPermissionSchemeGrants(_ string) error {
	// Grants are deleted when the parent scheme is deleted.
	// No standalone sweep needed.
	return nil
}

func sweepPermissionSchemes(_ string) error {
	client, err := testutil.SweepClient()
	if err != nil {
		return fmt.Errorf("getting sweep client: %w", err)
	}

	ctx := context.Background()

	// The permission scheme list endpoint uses a non-paginated "permissionSchemes" key,
	// NOT the standard "values" key used by GetAllPages. Decode manually.
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
				Config: fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Integration test (run %s)"
}
`, rName, testutil.RunID()),
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
	rNameUpdated := rName + "-upd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testCheckPermissionSchemeDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "original"
}
`, rName),
			},
			{
				Config: fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "updated"
}
`, rNameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "name", rNameUpdated),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "description", "updated"),
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
				Config: fmt.Sprintf(`
resource "atlassian_jira_permission_scheme" "test" {
  name = %q
}

resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id  = atlassian_jira_permission_scheme.test.id
  permission = "BROWSE_PROJECTS"
  holder_type = "anyone"
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "permission", "BROWSE_PROJECTS"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_type", "anyone"),
					resource.TestCheckResourceAttrSet("atlassian_jira_permission_scheme_grant.test", "id"),
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
```

- [ ] **Step 3: Read the actual permission scheme resource files and adjust**

After reading the resource files, verify:
- The `holder_type` and `permission` values are valid Jira permission scheme grant types
- The API endpoint for permission schemes may return data differently from `GetAllPages` (check if it uses a `permissionSchemes` top-level key instead of `values`)
- Adjust the sweeper's `GetAllPages` call if the permission scheme endpoint uses a non-standard paginated response

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build ./internal/jira/...`
Expected: No errors

- [ ] **Step 5: Verify tests are skipped without TF_ACC**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./internal/jira/... -run '^TestIntegrationPermissionScheme' -v -count=1`
Expected: All tests show `--- SKIP`

- [ ] **Step 6: Run linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./internal/jira/...`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add internal/jira/permission_scheme_resource_integration_test.go
git commit -m "test: add permission scheme integration tests and sweepers"
```

---

### Task 8: Final Verification and Push

**Files:** None (verification only)

- [ ] **Step 1: Run all mock tests to ensure nothing is broken**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go test ./... -v -count=1 -timeout 5m`
Expected: All existing tests PASS, all new `TestIntegration*` tests SKIP

- [ ] **Step 2: Run full linter**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && golangci-lint run --timeout=5m ./...`
Expected: No errors

- [ ] **Step 3: Run build**

Run: `cd /Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian && go build -o terraform-provider-atlassian`
Expected: Binary built successfully

- [ ] **Step 4: Push branch and create PR**

```bash
git push origin feat/integration-tests-phase1
```

Then create the PR:

```bash
gh pr create \
  --title "test: add integration test infrastructure and Phase 1 tests" \
  --base main \
  --head feat/integration-tests-phase1 \
  --body "$(cat <<'EOF'
## Summary

- Adds integration test infrastructure (shared helpers, CI workflow, sweepers)
- Adds Phase 1 integration tests for: group, project, issue type, permission scheme

## Changes

- `internal/testutil/integration.go` — `SkipIfNoAcc`, `SweepClient` (with tenant allowlist), `RunID`
- `.github/workflows/integration-test.yml` — runs on merge to main, protected `integration` environment
- `Makefile` — `testintegration` and `sweep` targets
- `internal/jira/integration_preflight_test.go` — verifies API access before suite
- `internal/jira/group_resource_integration_test.go` — 3 tests + sweeper
- `internal/jira/project_resource_integration_test.go` — 2 tests + sweeper
- `internal/jira/issue_type_resource_integration_test.go` — 4 tests + sweeper
- `internal/jira/permission_scheme_resource_integration_test.go` — 3 tests + sweepers

## Test plan

- [ ] All mock tests still pass (`go test ./... -count=1`)
- [ ] All integration tests skip without `TF_ACC=1`
- [ ] Linter passes
- [ ] Set up GitHub Environment `integration` with required reviewers
- [ ] Add secrets to `integration` environment
- [ ] Merge → integration tests run on main
EOF
)"
```
