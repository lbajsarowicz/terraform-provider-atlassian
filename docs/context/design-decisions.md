# Architecture Decision Records

## ADR-001: Direct HTTP client, no go-jira dependency
**Context:** fourplusone/jira depends on `andygrunwald/go-jira` which lacked `accountId` support, blocking critical fixes.
**Decision:** Build a thin HTTP client wrapping the Atlassian REST API directly.
**Consequences:** More initial code, but full control over API surface, pagination, and retries.

## ADR-002: accountId only, zero username support
**Context:** Atlassian deprecated `username` for GDPR compliance. Multiple fourplusone resources are broken because of this.
**Decision:** Only support `accountId` for user references. No backward compatibility with `username`.
**Consequences:** Clean API surface. Users must know accountId (discoverable via API/data sources).

## ADR-003: Plugin Framework, not legacy SDK
**Context:** Terraform Plugin Framework is the modern replacement for Plugin SDK v2. Better type safety, validation, plan modifiers.
**Decision:** Use `hashicorp/terraform-plugin-framework` exclusively. No SDK v2.
**Consequences:** Cannot reuse code from fourplusone (which uses SDK v2). Better long-term maintainability.

## ADR-004: GPL license
**Context:** User preference for copyleft licensing on this public provider.
**Decision:** GPL-3.0-or-later.
**Consequences:** Forks must also be GPL. Compatible with OpenTofu (MPL-2.0) ecosystem.

## ADR-005: Public from day 1
**Context:** Provider is intended to be a reusable community resource.
**Decision:** Public GitHub repo from day 1. `dev_overrides` for local development, OpenTofu registry after MVP validation.
**Consequences:** Code quality and documentation must be high from the start.

## ADR-006: Incremental import, not big-bang
**Context:** Codex review identified that importing all resources at once masks Read bugs.
**Decision:** Import each resource type into org-atlassian immediately after implementing it.
**Consequences:** Slower rollout but higher confidence in each resource.

## ADR-007: MVP = 3 resource families (group, project, permission_scheme)
**Context:** Codex review found 7-resource MVP was too large.
**Decision:** Prove the full loop (CRUD + import + drift detection + CI) with 3 resource families before scaling.
**Consequences:** Faster time-to-value, learn from first resources before building the rest.

## ADR-008: 404 on Read removes state, 404 on Delete is success
**Context:** Standard Terraform provider pattern. Resources deleted outside of Terraform should be gracefully handled.
**Decision:** All Read functions check for 404 and call `resp.State.RemoveResource()`. All Delete functions use `DeleteWithStatus` and silently succeed on 404.
**Consequences:** `tofu plan` after external deletion shows "will create" instead of erroring. `tofu destroy` never fails on already-deleted resources.

## ADR-009: Request body preservation on retry
**Context:** Codex found that Go's `http.Request.Body` is consumed on first read. Retries send empty body.
**Decision:** Buffer body with `bytes.NewReader` before retry loop, seek to start on each attempt.
**Consequences:** POST/PUT retries work correctly.

## ADR-010: Manual approval on tofu apply
**Context:** `auto_approve: true` in CI with full admin credentials is a security risk.
**Decision:** Add GitHub environment `production` with required reviewers for the apply workflow in org-atlassian.
**Consequences:** Merge to main triggers plan, but apply requires explicit approval.

## ADR-011: Feature-scoped PRs with Codex review gate
**Context:** Monolithic PRs make review difficult and delay feedback.
**Decision:** One PR per feature/resource family. Codex code review must pass before merge. No moving to next feature until current PR is clean.
**Consequences:** Slower throughput but higher quality. Each resource benefits from learnings of the previous review.

## ADR-012: Preserve plan values on Create (partial API responses)
**Context:** Jira POST endpoints return partial responses (e.g., project POST returns only id+key, not name/description).
**Decision:** After POST, only use response for server-generated fields (id, key). Keep plan values for everything the user specified.
**Consequences:** No state drift on first plan after apply.

## ADR-013: PathEscape for paths, QueryEscape for params
**Context:** `url.QueryEscape` encodes spaces as `+`, which is wrong for URL path segments.
**Decision:** Added `PathEscape` helper. All path construction uses `PathEscape`, all query params use `QueryEscape`.
**Consequences:** Correct URL encoding everywhere.

---

## Constraints

### Atlassian API Limitations
- **Automation rules:** No public REST API. Cannot manage via provider.
- **Board column config:** Read-only via API. Cannot set columns.
- **Workflow conditions/validators/post-functions:** Not fully exposed. Only structure (statuses + transitions) manageable.
- **Workflow scheme updates on active projects:** Creates draft, requires publish + poll.
- **Notification scheme updates:** Internal IDs change on modify (delete+recreate pattern).
- **Issue security scheme members:** API marked as EXPERIMENTAL.
- **Priority scheme API:** Recently added (~2024), may carry beta headers.
- **Confluence pagination:** Cursor-based (v2), different from Jira's offset-based.

### Technical Constraints
- **Rate limits:** Per-user per-endpoint. HTTP 429 with Retry-After header.
- **Eventual consistency:** Some operations have propagation delay.
- **ID instability:** Destroy+recreate gives new IDs. Cross-resource references cascade.
- **System defaults:** Default schemes cannot be deleted. Treat as data sources.
- **Ordering:** Custom field options, screen tab fields, priorities have meaningful display order. API uses relative move operations.
