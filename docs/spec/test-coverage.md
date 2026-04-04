# Test Coverage — Technical Spec

## Overview

This spec documents rules and patterns for achieving full unit + integration test coverage across all resources and data sources in the provider. All tests follow existing codebase patterns: `httptest` mocks for unit tests, real Atlassian API calls for integration tests, sweepers for cleanup.

## Mock JSON Correctness

All unit test mocks must match Go struct field types exactly:

- `int`, `int64` fields → JSON integer (not string)
- `string` fields → JSON string
- `json.Number` fields → JSON number (integer or string both accepted)

**Never mix types.** A struct field typed `int64` that receives a JSON string will silently deserialize to zero or fail. This is a frequent source of mock test false-positives where the resource appears to work but the mock state is incorrect.

## Paginated Mock Responses

Mocks serving paginated endpoints (`GetAllPages`) must include `"isLast": true` in the response. Without this, `GetAllPages` loops indefinitely and tests hang.

Single-page mock format:

```json
{
  "values": [...],
  "startAt": 0,
  "maxResults": 50,
  "total": N,
  "isLast": true
}
```

## Non-Standard Response Keys

Some Jira endpoints use non-standard top-level response keys (not `values`):

- `GET /rest/api/3/permissionscheme` returns `{"permissionSchemes": [...]}` (not `{"values": [...]}`)

Mock servers for any code path that calls this endpoint (e.g., `project_permission_scheme` Delete — which calls it to find the default scheme) must use `permissionSchemes` as the key. Using `values` will cause the list to deserialize as empty, leaving the default-scheme-detection path untested.

## Integration Test Resource Naming

- **Resource names**: `tf-acc-test-` prefix via `acctest.RandomWithPrefix("tf-acc-test")`
- **Project keys**: `TFACC` prefix + random uppercase letters
- Sweepers filter by `tf-acc-test-` prefix to identify test-created resources

## No Hardcoded Default IDs

Integration test destroy checks must never assert a specific default scheme/workflow ID. Default IDs vary per Atlassian tenant. Instead, assert that the test resource's ID is no longer assigned to the object under test.

**Wrong**:
```go
assert.Equal(t, 10000, projectSchemeID) // hardcoded Cloud default
```

**Correct**:
```go
assert.NotEqual(t, testSchemeID, projectSchemeID) // test scheme no longer assigned
```

## Sweeper Dependency Chain

Sweepers must declare dependencies so resources are deleted in the correct order. Example for permission scheme associations:

```
atlassian_jira_project_permission_scheme  (swept first — association)
  ↑ depended on by
atlassian_jira_permission_scheme_grant    (swept second — grants within scheme)
  ↑ depended on by
atlassian_jira_permission_scheme          (swept third — the scheme itself)
  ↑ depended on by
atlassian_jira_project                   (swept last — the project root)
```

Full dependency order (outermost → innermost):

1. Project associations: `project_permission_scheme`, `project_issue_type_scheme`, `project_workflow_scheme`, `project_issue_type_screen_scheme`
2. Sub-scheme resources: `permission_scheme_grant`, `project_role_actor`
3. Schemes: `permission_scheme`, `issue_type_scheme`, `workflow_scheme`, `issue_type_screen_scheme`, `screen_scheme`
4. Screen hierarchy: `screen_tab_field` → `screen_tab` → `screen`
5. Building blocks: `workflow`, `status`, `issue_type`, `custom_field`
6. Fundamentals: `project_role`, `project`, `group`

Adding `atlassian_jira_project` to the `atlassian_jira_permission_scheme` sweeper's `Dependencies` list is required to prevent "scheme is in use" 400 errors during cleanup.

## Bug: `permission_scheme_grant_resource` ImportState

`ImportState` does not set `holder_parameter` to `types.StringNull()` when the API returns an empty parameter (e.g., `anyone`, `reporter` holder types). This causes `ImportStateVerify` to fail because post-import state has an unset attribute while post-Read state has an explicit null.

**Fix** (add `else` branch at line 273-275 of `permission_scheme_grant_resource.go`):

```go
if result.Holder.Parameter != "" {
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("holder_parameter"), result.Holder.Parameter)...)
} else {
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("holder_parameter"), types.StringNull())...)
}
```

After applying this fix, remove `ImportStateVerifyIgnore: []string{"scheme_id"}` from the grant integration test — the import correctly sets `scheme_id` and the ignore was masking a now-fixed bug.

## Project Association Resource Patterns

### project_permission_scheme

- API assign: `PUT /rest/api/3/project/{key}/permissionscheme` — body `{"id": int}` (integer, not string)
- API read: `GET /rest/api/3/project/{key}/permissionscheme`
- API delete (find default): `GET /rest/api/3/permissionscheme` → `permissionSchemes` key
- Destroy check: GET project's permission scheme, assert assigned scheme ID is NOT the test scheme ID

### project_workflow_scheme

- API assign: `PUT /rest/api/3/workflowscheme/project` — body `{"workflowSchemeId": str, "projectId": str}`
- API read: `GET /rest/api/3/workflowscheme/project?projectId={id}` — paginated `PageResponse` format with `values[]`, `startAt`, `maxResults`, `isLast`
- Mock constraint: `workflowScheme.id` must be JSON integer (Go type `int64`); `projectId` is string
- Delete is a documented no-op — unit test verifies no API call is made on DELETE

### project_issue_type_scheme

- Unit test exists — fix existing `CheckDestroy` stub to actually verify the mock state shows scheme reverted to default after destroy
- Destroy check (integration): GET project's issue type scheme, verify it is no longer the test scheme

## Data Source Unit Test Patterns

### Common pattern

For each data source, unit tests cover:
1. Found by name — verify all computed attributes
2. API 404 or empty result — verify error diagnostic
3. API 200 with name mismatch — verify "not found" error (where applicable)

Integration tests: create the parent resource first (never rely on built-in defaults — names vary per tenant), look up via data source by name, verify all attributes match.

### Per-data-source details

**custom_field_data_source**
- API: `GET /rest/api/3/field/search?query={name}&type=custom`
- Response key: `values[]` (not paginated — single page only)
- Known gap: does not paginate; add unit test with `isLast: false` and target field absent to document the limitation

**issue_type_scheme_data_source**
- API: `GET /rest/api/3/issuetypescheme` (paginated) + `GET /rest/api/3/issuetypescheme/mapping?issueTypeSchemeId={id}`
- Mock trap: mapping response items must have `issueTypeSchemeId` matching the scheme under test, otherwise silently filtered and `issue_type_ids` is empty

**issue_type_screen_scheme_data_source**
- API: `GET /rest/api/3/issuetypescreenscheme` (paginated via GetAllPages)

**screen_data_source**
- API: `GET /rest/api/3/screens?maxResults=100&startAt={n}` (manual pagination)

**screen_scheme_data_source**
- API: `GET /rest/api/3/screenscheme` (paginated via GetAllPages)
- Mock note: `screens` field values must be JSON integers (Go helper converts int64 → string)

**workflow_data_source**
- API: `GET /rest/api/3/workflow/search?workflowName={name}&expand=statuses`
- Two not-found paths: API 404 (status code), and API 200 with name mismatch in values — both must be tested

**workflow_scheme_data_source**
- API: `GET /rest/api/3/workflowscheme` (paginated via GetAllPages)

## project_role_actor Integration Test

- Import composite ID format: `{projectKey}/{roleId}/{actorType}/{actorValue}` where `actorType` is the literal string `atlassianUser` or `atlassianGroup` (e.g., `MYPROJ/10100/atlassianUser/5a1234abc`)
- Fetch the authenticated user's account ID via `GET /rest/api/3/myself` — never hardcode
- Look up the built-in "Administrators" role via `GET /rest/api/3/role` to get its ID — never hardcode
- Destroy check: `GET /rest/api/3/project/{key}/role/{roleId}`, iterate actors array, verify the test actor entry is removed
- No standalone sweeper — actors are cleaned up when projects are swept
