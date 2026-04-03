# Integration Tests Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **IMPORTANT:** Subagents MUST use the **Sonnet** model, not Opus.

**Goal:** Add Phase 3 integration tests for the screen chain: screen (+tab +field), screen scheme, and issue type screen scheme (+project association).

**Architecture:** Same as Phase 1/2. Tests are `*_integration_test.go` files with `TestIntegration` prefix, gated on `TF_ACC=1`.

**Tech Stack:** Go 1.25, terraform-plugin-framework v1.19.0, terraform-plugin-testing v1.15.0

**Working directory:** `/Users/lbajsarowicz/lbajsarowicz/terraform-provider-atlassian`

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `internal/jira/screen_resource_integration_test.go` | Integration tests + sweeper for screen, screen_tab, screen_tab_field |
| **Create:** `internal/jira/screen_scheme_resource_integration_test.go` | Integration tests + sweeper for screen_scheme |
| **Create:** `internal/jira/issue_type_screen_scheme_resource_integration_test.go` | Integration tests + sweepers for issue_type_screen_scheme + project association |

---

## Task 1: Screen (+Tab +Field) Integration Tests

Create `internal/jira/screen_resource_integration_test.go`. Key points:
- Screen: `id` (computed, int64→string), `name` (required), `description` (optional, default "")
- Screen tab: `id` (computed), `screen_id` (required, ForceNew), `name` (required). Import: `{screen_id}/{tab_id}`
- Screen tab field: `screen_id`, `tab_id`, `field_id` (all required, ForceNew, no `id`). Import: `{screen_id}/{tab_id}/{field_id}`
- Screen read is paginated via custom loop (not GetAllPages — uses `startAt`/`maxResults`)
- Screen ID is `int64` in API
- Tab read returns flat array
- Field read returns flat array

Sweeper: screen only (tabs/fields cascade). Must depend on `screen_scheme`.

Tests: screen basic+update, screen tab basic (with import), screen tab field basic (with import), screen data source is NOT available (no data source exists for screen).

## Task 2: Screen Scheme Integration Tests

Create `internal/jira/screen_scheme_resource_integration_test.go`. Key points:
- `id` (computed), `name` (required), `description` (optional, default ""), `screens` (map[string]string, required)
- `screens` map keys: "default" (required), "create"/"view"/"edit" (optional)
- API stores screens as `map[string]int64`, TF as `map[string]string`
- Read via `GetAllPages` at `/rest/api/3/screenscheme`
- Delete: `DELETE /rest/api/3/screenscheme/{id}`
- Import by ID

Sweeper: depends on `issue_type_screen_scheme`.

## Task 3: Issue Type Screen Scheme Integration Tests

Create `internal/jira/issue_type_screen_scheme_resource_integration_test.go`. Key points:
- `id` (computed), `name` (required), `description` (optional, default ""), `mappings` (map[string]string, required)
- `mappings` keys: "default" (required) + optional issue type IDs → screen scheme IDs
- Create returns `{"issueTypeScreenSchemeId": "..."}`
- Read: list + mapping sub-endpoint
- Delete: `DELETE /rest/api/3/issuetypescreenscheme/{id}`
- Import by ID. Design spec: `mappings` in ImportStateVerifyIgnore (empty map vs null)
- Project association: `project_id` + `issue_type_screen_scheme_id`, delete reverts to scheme ID "1"

Sweepers: project_issue_type_screen_scheme (no-op), issue_type_screen_scheme (depends on project assoc).

## Task 4: Final Verification and Push

Run all tests, lint, build. Push and create PR.
