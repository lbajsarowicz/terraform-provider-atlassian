# Test Coverage Gaps — Design Spec

## Goal

Achieve full unit + integration test coverage for all resources and data sources in the `terraform-provider-atlassian` provider. 14 test gaps across 5 resources and 8 data sources, plus 1 resource bug fix.

## Architecture

4 independent, sequentially-mergeable PRs grouped by API domain. Each PR is self-contained with green CI. All tests follow existing codebase patterns: `httptest` mocks for unit tests, real Atlassian API calls for integration tests, sweepers for cleanup.

## Tech Stack

- Go 1.25, `hashicorp/terraform-plugin-testing` v1.15.0
- `httptest.NewServer` for unit test mocking
- `testutil.SkipIfNoAcc(t)` / `testutil.SweepClient()` for integration tests
- `acctest.RandString` for unique resource names with `tf-acc-test-` prefix

---

## PR 1: Permission Scheme Grant Fixes

**Scope**: Bug fix + test gap fill. Most tests already exist.

### Bug Fix: `permission_scheme_grant_resource.go` ImportState

`ImportState` does not set `holder_parameter` to `types.StringNull()` when the API returns empty parameter (e.g. `anyone`, `reporter` holder types). This causes `ImportStateVerify` to fail because post-import state has unset attribute while post-Read state has explicit null.

**Fix**: Add `else` branch at line 273-275:
```go
if result.Holder.Parameter != "" {
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("holder_parameter"), result.Holder.Parameter)...)
} else {
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("holder_parameter"), types.StringNull())...)
}
```

### Test Fix: Remove ImportStateVerifyIgnore

Remove `ImportStateVerifyIgnore: []string{"scheme_id"}` from the grant integration test. The `ImportState` correctly sets `scheme_id` — the ignore was suppressing verification unnecessarily.

### Test Fix: Add Grant Destroy Check

Current `testCheckPermissionSchemeDestroyed` only checks parent scheme, not grants. Add a dedicated destroy check that:
1. Iterates resources of type `atlassian_jira_permission_scheme_grant`
2. Extracts `scheme_id` from `rs.Primary.Attributes["scheme_id"]` and grant ID from `rs.Primary.ID`
3. Calls `GET /rest/api/3/permissionscheme/{scheme_id}/permission/{grant_id}`
4. Asserts 404

### Test Fix: Sweeper Dependencies

Add `atlassian_jira_project` to the `atlassian_jira_permission_scheme` sweeper dependency list. This ensures projects are swept before schemes, preventing "scheme is in use" 400 errors during cleanup.

### Files Modified
- `internal/jira/permission_scheme_grant_resource.go` — ImportState fix
- `internal/jira/permission_scheme_resource_integration_test.go` — destroy check, ImportStateVerifyIgnore, sweeper deps

---

## PR 2: Project Association Resources

**Scope**: 3 resources that assign schemes to projects. Unit + integration tests.

### project_permission_scheme_resource

**Unit test** (`project_permission_scheme_resource_test.go`):
- httptest mock serving:
  - `PUT /rest/api/3/project/{key}/permissionscheme` — assign scheme (body: `{"id": int}`)
  - `GET /rest/api/3/project/{key}/permissionscheme` — read current assignment
  - `GET /rest/api/3/permissionscheme` — list schemes (used by Delete to find default)
- Scenarios:
  - Basic create + read + verify attributes
  - Import by project key
  - Read not-found (project returns 404 → RemoveResource)
- Mock constraints: scheme `id` must be JSON integer, not string
- Destroy check in unit test: verify mock state shows scheme reverted to default (not a no-op stub)

**Integration test** (`project_permission_scheme_resource_integration_test.go`):
- Creates `atlassian_jira_project` + `atlassian_jira_permission_scheme` as dependencies
- Assigns scheme to project
- Steps: create + verify, import by project key
- Destroy check: GET project's permission scheme, assert assigned scheme ID is NOT the test scheme ID (cannot hardcode the default scheme ID — varies per tenant)
- Sweeper: no standalone sweeper; project deletion cascades

### project_workflow_scheme_resource

**Unit test** (`project_workflow_scheme_resource_test.go`):
- httptest mock serving:
  - `PUT /rest/api/3/workflowscheme/project` — assign scheme (body: `{"workflowSchemeId": str, "projectId": str}`)
  - `GET /rest/api/3/workflowscheme/project?projectId={id}` — paginated read (uses offset-based `PageResponse` format with `values[]`, `startAt`, `maxResults`, `isLast`)
- Mock constraints: `workflowScheme.id` must be JSON integer (Go type is `int64`), `projectId` is string
- Scenarios:
  - Basic create + read + verify attributes
  - Import by project ID
  - Read not-found (empty values → RemoveResource)
- Delete is a documented no-op — unit test verifies no API call is made on DELETE

**Integration test** (in same file or shared `project_association_integration_test.go`):
- Creates `atlassian_jira_project` + `atlassian_jira_workflow_scheme` as dependencies
- Assigns scheme to project
- Steps: create + verify, import by project ID
- Destroy check: call `GET /rest/api/3/workflowscheme/project?projectId={id}` via Jira API directly, assert workflow scheme is STILL assigned (confirming no cleanup was attempted). Terraform state removal is verified automatically by the framework.

### project_issue_type_scheme_resource

**Unit test exists** (`project_issue_type_scheme_resource_test.go`) — already covers basic create, import, not-found.

**Fix existing unit test**: Replace no-op `CheckDestroy` stub with actual verification that mock state shows scheme reverted to default ID 10000 after destroy. This is the whole point of the resource's Delete behavior.

**Integration test** (in same association integration file):
- Creates `atlassian_jira_project` + `atlassian_jira_issue_type_scheme` as dependencies
- Assigns scheme to project
- Steps: create + verify, import by project ID
- Destroy check: GET project's issue type scheme, verify it is no longer the test scheme. Comment noting that Cloud default is typically ID 10000.

### Sweeper Strategy
No standalone sweepers for association resources. Project sweeper handles cleanup. All integration tests depend on `atlassian_jira_project` sweeper.

### Files Created/Modified
- Create: `internal/jira/project_permission_scheme_resource_test.go`
- Create: `internal/jira/project_workflow_scheme_resource_test.go`
- Create: `internal/jira/project_association_integration_test.go` (or separate files per resource)
- Modify: `internal/jira/project_issue_type_scheme_resource_test.go` — fix CheckDestroy stub

---

## PR 3: Project Role Actor Integration Test

**Scope**: Integration test only. Unit tests already exist covering all actor types, drift detection, and import.

**Integration test** (`project_role_actor_resource_integration_test.go`):
- Creates `atlassian_jira_project` as dependency
- Fetches authenticated user's account ID via `GET /rest/api/3/myself`
- Looks up the built-in "Administrators" role via `GET /rest/api/3/role` to get its ID
- Adds user as actor to the project role
- Steps: create + verify attributes (`actor_type`, `actor_value`, `project_key`, `role_id`), import with composite ID `{projectKey}/{roleId}/{actorType}/{actorValue}` where `actorType` is the literal string `atlassianUser` or `atlassianGroup` (e.g. `MYPROJ/10100/atlassianUser/5a1234abc`)
- Destroy check: `GET /rest/api/3/project/{key}/role/{roleId}`, iterate actors array, verify the test user's actor entry is removed
- No standalone sweeper — actors are cleaned up when projects are swept

### Files Created
- Create: `internal/jira/project_role_actor_resource_integration_test.go`

---

## PR 4: Remaining Data Sources (7)

**Scope**: Unit + integration tests for 7 data sources. All follow the same pattern.

### Common Pattern

**Unit test** (per data source):
- httptest mock returning appropriate JSON structure
- Scenario 1: Found by name — verify all computed attributes
- Scenario 2: API 404 or empty result — verify error diagnostic
- Scenario 3 (where applicable): API 200 with name mismatch — verify "not found" error

**Integration test** (per data source):
- Create parent resource first (never rely on built-in defaults — names vary per tenant)
- Look up via data source by name
- Verify all attributes match

### Per-Data-Source Details

**custom_field_data_source** (`custom_field_data_source_test.go`, integration in same or separate file):
- API: `GET /rest/api/3/field/search?query={name}&type=custom`
- Response key: `values[]` (not paginated — single page only)
- Unit scenarios: found (exact name match among fuzzy results), not-found (empty values), name-mismatch (API returns fields but none match exactly)
- Known limitation: does not paginate. Add unit test with `isLast: false` and target field absent → documents the gap.
- Integration test: create a custom field via `atlassian_jira_custom_field` resource, then look up by name via data source

**issue_type_scheme_data_source** (`issue_type_scheme_data_source_test.go`):
- API: `GET /rest/api/3/issuetypescheme` (paginated via GetAllPages) + `GET /rest/api/3/issuetypescheme/mapping?issueTypeSchemeId={id}` (for ordered items)
- Mock trap: mapping response items must have `issueTypeSchemeId` matching the scheme under test, otherwise silently filtered out and `issue_type_ids` will be empty
- Unit scenarios: found (verify id, name, description, issue_type_ids ordering), not-found
- Integration test: create issue type scheme, look up by name, verify issue_type_ids list

**issue_type_screen_scheme_data_source** (`issue_type_screen_scheme_data_source_test.go`):
- API: `GET /rest/api/3/issuetypescreenscheme` (paginated via GetAllPages)
- Unit scenarios: found, not-found
- Integration test: create issue type screen scheme, look up by name

**screen_data_source** (`screen_data_source_test.go`):
- API: `GET /rest/api/3/screens?maxResults=100&startAt={n}` (manual pagination)
- Unit scenarios: found (verify id, name, description), not-found
- Integration test: create screen, look up by name

**screen_scheme_data_source** (`screen_scheme_data_source_test.go`):
- API: `GET /rest/api/3/screenscheme` (paginated via GetAllPages)
- Mock note: `screens` field values must be JSON integers (Go helper converts int64 → string)
- Unit scenarios: found (verify id, name, description, screens map), not-found
- Integration test: create screen scheme, look up by name, verify screens map

**workflow_data_source** (`workflow_data_source_test.go`):
- API: `GET /rest/api/3/workflow/search?workflowName={name}&expand=statuses`
- Two not-found paths: API 404 (status code), and API 200 with name mismatch in values
- Unit scenarios: found (verify id, name, description, statuses list), API 404, 200-with-name-mismatch
- Integration test: create workflow, look up by name, verify statuses

**workflow_scheme_data_source** (`workflow_scheme_data_source_test.go`):
- API: `GET /rest/api/3/workflowscheme` (paginated via GetAllPages)
- Unit scenarios: found (verify id, name, description, default_workflow, issue_type_mappings), not-found
- Integration test: create workflow scheme, look up by name, verify default_workflow and issue_type_mappings

### Files Created
- Create: 7 unit test files (`*_data_source_test.go`)
- Create: 1 integration test file (`data_source_integration_test.go`) or 7 separate files

---

## Cross-Cutting Concerns

### Mock JSON Correctness
All unit test mocks must match Go struct types:
- `int`, `int64` fields → JSON integer (not string)
- `string` fields → JSON string
- `json.Number` fields → JSON number (integer or string both work)

### Paginated Mock Responses
Mocks serving paginated endpoints (`GetAllPages`) must include `"isLast": true` in the response. Without this, `GetAllPages` loops indefinitely and tests hang. For single-page mocks: `{"values": [...], "startAt": 0, "maxResults": 50, "total": N, "isLast": true}`.

### Non-Standard Response Keys
Some Jira endpoints use non-standard response keys:
- `GET /rest/api/3/permissionscheme` returns `{"permissionSchemes": [...]}` (not `{"values": [...]}`)
- Mock servers for `project_permission_scheme` Delete (which calls this endpoint to find the default scheme) must use the `permissionSchemes` key or the list will deserialize as empty, leaving the default-scheme-detection path untested.

### Integration Test Resource Naming
All test resources use `tf-acc-test-` prefix for sweeper filtering. Project keys use `TFACC` prefix + random uppercase letters.

### Sweeper Dependency Chain
```
atlassian_jira_project (root — swept last)
  ↑ depends on
atlassian_jira_permission_scheme
  ↑ depends on
atlassian_jira_permission_scheme_grant
  ↑ depends on
atlassian_jira_project_permission_scheme
```

### No Hardcoded Default IDs
Integration test destroy checks must never assert a specific default scheme/workflow ID. Instead, assert the test resource's ID is no longer assigned.
