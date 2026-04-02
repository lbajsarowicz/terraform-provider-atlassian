# Integration Tests for terraform-provider-atlassian

## Goal

Add real-API integration tests that run against `lbajsarowicz.atlassian.net` on every merge to `main`, with sweeper-based cleanup for stale resources. Phased rollout covers all 21 resources across 4 phases.

## Architecture

Integration tests live alongside existing mock tests as `*_integration_test.go` files in `internal/jira/`. They are gated on `TF_ACC=1` and use the `TestIntegration` prefix to avoid collision with existing `TestAcc` mock tests. A new CI workflow runs them on merge to `main` only, within a protected GitHub Environment (`integration`) that requires reviewer approval. Sweeper functions clean up leaked test resources.

## Test File Structure

Each resource family gets one integration test file next to its mock test file:

```
internal/jira/
  group_resource_test.go                    # existing mock tests (TestAcc*)
  group_resource_integration_test.go        # NEW: real API tests (TestIntegration*)
  project_resource_test.go
  project_resource_integration_test.go
  ...
```

## Naming Conventions

- **Test functions**: `TestIntegration<Resource>_<scenario>` (e.g., `TestIntegrationGroupResource_basic`)
- **Resource names**: `acctest.RandomWithPrefix("tf-acc-test")` — e.g., `tf-acc-test-abc123`
- **Sweeper prefix match**: `tf-acc-test-`
- **Run traceability**: When `GITHUB_RUN_ID` is set, include a truncated run ID in the resource description field for traceability (e.g., `"integration test run 12345678"`). Sweepers can use this to distinguish active vs stale resources.

The `TestIntegration` prefix serves two purposes:
1. CI can target them with `-run '^TestIntegration'` (anchored regex) without running mock `TestAcc*` tests
2. Clear visual distinction from mock tests

## Shared Test Infrastructure

### `internal/testutil/integration.go`

New file providing:

```go
// SkipIfNoAcc skips the test unless TF_ACC=1 is set.
func SkipIfNoAcc(t *testing.T) {
    if os.Getenv("TF_ACC") == "" {
        t.Skip("TF_ACC not set, skipping integration test")
    }
}

// AllowedHosts is the set of Jira instances that sweepers are allowed to operate on.
var AllowedHosts = map[string]bool{
    "lbajsarowicz.atlassian.net": true,
}

// SweepClient returns a real atlassian.Client using environment variables.
// Validates that the target host is in the AllowedHosts allowlist and that
// ATLASSIAN_SWEEP_CONFIRM matches the hostname (for local safety).
// In CI (GITHUB_ACTIONS=true), the confirmation env var is not required.
func SweepClient() (*atlassian.Client, error) {
    client, err := atlassian.NewClient(atlassian.ClientConfig{})
    if err != nil {
        return nil, fmt.Errorf("creating sweep client: %w", err)
    }

    host := extractHost(os.Getenv("ATLASSIAN_URL"))
    if !AllowedHosts[host] {
        return nil, fmt.Errorf("sweep aborted: host %q is not in the allowlist", host)
    }

    if os.Getenv("GITHUB_ACTIONS") != "true" {
        confirm := os.Getenv("ATLASSIAN_SWEEP_CONFIRM")
        if confirm != host {
            return nil, fmt.Errorf(
                "sweep aborted: set ATLASSIAN_SWEEP_CONFIRM=%s to confirm local sweep", host)
        }
    }

    return client, nil
}

// RunID returns a truncated GitHub run ID for resource tagging, or "local" for local runs.
func RunID() string {
    if id := os.Getenv("GITHUB_RUN_ID"); id != "" && len(id) >= 8 {
        return id[:8]
    }
    return "local"
}
```

### Preflight Test

A single preflight test verifies API access and permissions before the suite runs:

```go
func TestIntegration_preflight(t *testing.T) {
    testutil.SkipIfNoAcc(t)
    client, err := testutil.SweepClient()
    if err != nil {
        t.Fatalf("preflight: cannot create client: %s", err)
    }
    // Verify we can list projects (requires admin-level read access)
    var result interface{}
    _, err = client.GetWithStatus(ctx, "/rest/api/3/project/search?maxResults=1", &result)
    if err != nil {
        t.Fatalf("preflight: API access failed — check credentials and permissions: %s", err)
    }
}
```

### Sweeper Registration

Each integration test file registers sweepers in `init()`:

```go
func init() {
    resource.AddTestSweepers("atlassian_jira_group", &resource.Sweeper{
        Name: "atlassian_jira_group",
        F:    sweepGroups,
    })
}
```

Sweepers list all resources of their type via the Jira API, filter by `tf-acc-test-` prefix, and delete matches. Sweepers declare dependencies to ensure correct deletion order (e.g., project associations deleted before schemes). Each sweeper uses `context.WithTimeout` (2 minutes per resource type) and retries "resource still in use" errors with backoff.

## CI Workflow

### `.github/workflows/integration-test.yml`

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

Key decisions:
- **`environment: integration`**: GitHub Environment with required reviewers. Protects secrets from malicious code merged to main. Also gates `workflow_dispatch` triggers.
- **`concurrency` group with `cancel-in-progress: false`**: Prevents parallel runs against the same Jira instance, which could cause flaky tests from resource name collisions. Queues runs instead of cancelling.
- **`-run '^TestIntegration'`**: Anchored regex — only runs `TestIntegration*` functions, not existing `TestAcc*` mock tests or any helper function that happens to contain "Integration".
- **Sweeper runs `always()`**: Cleans up even when tests fail.
- **`workflow_dispatch`**: Allows manual trigger for debugging (gated by environment reviewers).
- **Not on PRs**: Secrets are not exposed to fork PRs (security).
- **45-minute job timeout, 30-minute test timeout**: Leaves 15 minutes for sweepers and accounts for Jira API throttling.

### Required GitHub Setup

**Secrets** (set on the `integration` environment):

| Secret | Value | Source |
|--------|-------|--------|
| `ATLASSIAN_URL` | `https://lbajsarowicz.atlassian.net` | 1Password |
| `ATLASSIAN_USER` | Account email | 1Password `atlassian-api-token/email` |
| `ATLASSIAN_TOKEN` | API token | 1Password `atlassian-api-token/api-token` |

**Environment** (`integration`):
- Required reviewers: repository owner (`lbajsarowicz`)
- Deployment branches: `main` only

**Branch protection** (already in place):
- Require PR reviews before merge
- No direct pushes to `main`

## Integration Test Pattern

Each test follows the standard Terraform acceptance test lifecycle but hits the real Jira API:

```go
func TestIntegrationGroupResource_basic(t *testing.T) {
    testutil.SkipIfNoAcc(t)
    rName := acctest.RandomWithPrefix("tf-acc-test")

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
        CheckDestroy:             testCheckGroupDestroyed,
        Steps: []resource.TestStep{
            {
                Config: testIntegrationGroupConfig(rName),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", rName),
                    resource.TestCheckResourceAttrSet("atlassian_jira_group.test", "id"),
                ),
            },
            {
                ResourceName:      "atlassian_jira_group.test",
                ImportState:        true,
                ImportStateVerify:  true,
            },
        },
    })
}
```

### CheckDestroy functions

Unlike mock tests (which use no-op CheckDestroy), integration tests verify the resource was actually deleted from Jira. CheckDestroy must distinguish 404 (resource gone — success) from transport/auth errors (which must fail the check):

```go
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
        statusCode, err := client.GetWithStatus(ctx, "/rest/api/3/group?groupId="+rs.Primary.ID, &result)
        if err != nil {
            return fmt.Errorf("error checking group %s destruction: %w", rs.Primary.ID, err)
        }
        if statusCode != http.StatusNotFound {
            return fmt.Errorf("group %s still exists (status %d)", rs.Primary.ID, statusCode)
        }
    }
    return nil
}
```

### Test scenarios per resource

- **`_basic`**: Create, verify attributes, import state verification
- **`_update`**: Modify attributes, verify update applied
- **`_disappears`**: Only for resources that support hard delete. Delete out-of-band via API, verify next plan detects removal. Skip for: project associations (can't unassign), default schemes.

### ImportStateVerifyIgnore

Some Jira resources normalize or server-populate fields. Per-resource ignore list:

| Resource | Ignored fields | Reason |
|----------|---------------|--------|
| `jira_workflow` | `statuses` | Server may reorder status references |
| `jira_workflow_scheme` | `issue_type_mappings` | Empty map vs null distinction |
| `jira_issue_type_screen_scheme` | `mappings` | Empty map vs null distinction |

Other resources should import cleanly. If a new field causes import drift, add it to this table and document why.

## Sweeper Design

### Dependency Order

Sweepers declare dependencies so resources are deleted in the correct order:

```
project associations (permission scheme, issue type scheme, workflow scheme, issue type screen scheme)
  → schemes (permission, issue type, workflow, screen, issue type screen)
    → building blocks (issue types, statuses, workflows, screens + tabs + fields)
      → fundamentals (groups, projects, custom fields, project roles)
```

Specifically:
1. `project_permission_scheme`, `project_issue_type_scheme`, `project_workflow_scheme`, `project_issue_type_screen_scheme`
2. `permission_scheme_grant`, `project_role_actor`
3. `permission_scheme`, `issue_type_scheme`, `workflow_scheme`, `issue_type_screen_scheme`, `screen_scheme`
4. `screen_tab_field` → `screen_tab` → `screen`
5. `workflow`, `status`, `issue_type`, `custom_field`
6. `project_role`, `project`, `group`

### Retry logic

Sweepers retry delete operations that fail with "resource still in use" (HTTP 409 or specific Jira error codes) up to 3 times with 5-second backoff. This handles Jira Cloud's eventual consistency where detaching a scheme from a project may not propagate instantly.

### Per-sweeper timeout

Each sweeper function uses `context.WithTimeout(ctx, 2*time.Minute)` to prevent a single sweeper from consuming the entire sweep budget.

### Local safety

Sweepers require `ATLASSIAN_SWEEP_CONFIRM=lbajsarowicz.atlassian.net` when run locally (outside GitHub Actions). This prevents accidental sweeps against a production instance if env vars are misconfigured.

## Phased Rollout

### Phase 1: Foundation (CI + core resources)

**Files created:**
- `.github/workflows/integration-test.yml`
- `internal/testutil/integration.go`
- `internal/jira/group_resource_integration_test.go`
- `internal/jira/project_resource_integration_test.go`
- `internal/jira/issue_type_resource_integration_test.go`
- `internal/jira/permission_scheme_resource_integration_test.go` (includes grant + project assoc)

**Tests per resource:**
- `_basic`: Create, verify attributes, import state
- `_update`: Modify attributes, verify update
- `_disappears`: Delete out-of-band, verify plan handles it (skip for project_permission_scheme)

**Sweepers:** group, project, issue_type, permission_scheme, permission_scheme_grant, project_permission_scheme

### Phase 2: Scheme chain

**Files created:**
- `internal/jira/issue_type_scheme_resource_integration_test.go` (includes project assoc)
- `internal/jira/workflow_resource_integration_test.go`
- `internal/jira/status_resource_integration_test.go`
- `internal/jira/workflow_scheme_resource_integration_test.go` (includes project assoc)

**Sweepers:** issue_type_scheme, project_issue_type_scheme, workflow, status, workflow_scheme, project_workflow_scheme

### Phase 3: Screen chain

**Files created:**
- `internal/jira/screen_resource_integration_test.go` (includes tab + field)
- `internal/jira/screen_scheme_resource_integration_test.go`
- `internal/jira/issue_type_screen_scheme_resource_integration_test.go` (includes project assoc)

**Sweepers:** screen (+ tab + field), screen_scheme, issue_type_screen_scheme, project_issue_type_screen_scheme

### Phase 4: Remaining resources

**Files created:**
- `internal/jira/custom_field_resource_integration_test.go`
- `internal/jira/project_role_resource_integration_test.go` (includes actor)

**Sweepers:** custom_field, project_role, project_role_actor

## Test Execution

### Local development

```bash
# Run all integration tests locally (requires env vars + sweep confirmation)
export ATLASSIAN_SWEEP_CONFIRM=lbajsarowicz.atlassian.net
TF_ACC=1 go test ./internal/jira/... -v -count=1 -timeout 30m -run '^TestIntegration'

# Run integration tests for a specific resource
TF_ACC=1 go test ./internal/jira/... -v -count=1 -run '^TestIntegrationGroupResource'

# Run sweepers to clean up stale test resources (requires confirmation)
ATLASSIAN_SWEEP_CONFIRM=lbajsarowicz.atlassian.net go test ./internal/jira/... -v -sweep=all -timeout 10m

# Run mock tests only (default, no TF_ACC needed)
go test ./internal/jira/... -v -count=1
```

### CI

Automatic on merge to `main` (gated by `integration` environment reviewers). Manual trigger via `workflow_dispatch` in GitHub Actions UI (also gated).

## What This Does NOT Cover

- Confluence resources (future phase)
- Provider registry publication
- Performance/load testing
- Multi-tenant testing (single demo instance only)
