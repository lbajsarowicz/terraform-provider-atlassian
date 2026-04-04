# terraform-provider-atlassian

Custom OpenTofu/Terraform provider for managing Atlassian Cloud (Jira + Confluence) configuration as code.

## Repository Goal

Provide a production-grade OpenTofu provider that enables declarative management of Atlassian Cloud configuration — the equivalent of `integrations/github` for Atlassian.

## Architecture

- **Language:** Go 1.23+
- **Framework:** `hashicorp/terraform-plugin-framework` (NOT legacy SDK v2)
- **API:** Jira Cloud REST API v3 + Confluence Cloud REST API v2
- **Auth:** Basic auth (email:api-token), env vars `ATLASSIAN_URL`, `ATLASSIAN_USER`, `ATLASSIAN_TOKEN`
- **Registry:** `registry.terraform.io/lbajsarowicz/atlassian` (published to Terraform Registry; OpenTofu consumes it natively)
- **License:** GPL-3.0-or-later

## Key Decisions

- **Dual-registry publishing:** Published to Terraform Registry (`registry.terraform.io/lbajsarowicz/atlassian`); OpenTofu consumes Terraform Registry providers natively, so no separate OpenTofu Registry publish is required
- **Release Please** (`googleapis/release-please-action@v4`): Automated versioning and changelog generation from conventional commits; creates release PRs automatically
- **GoReleaser v2:** Cross-compilation for all target platforms, GPG signing of release artifacts, and GitHub Release publishing on tag push

## Key Conventions

### Code
- `accountId` only — NEVER use `username` (deprecated by Atlassian GDPR)
- Every resource MUST implement: Create, Read, Update, Delete, ImportState
- Every Read MUST handle 404 → `resp.State.RemoveResource()` (deleted out-of-band)
- Every Delete MUST tolerate 404 via `DeleteWithStatus` (already gone = success)
- Every resource MUST have a corresponding data source
- Every attribute MUST have a read implementation (for drift detection)
- Direct HTTP client in `internal/atlassian/` — no third-party Jira SDK
- Request body MUST be buffered for retry support (bytes.NewReader pattern)
- `User-Agent: terraform-provider-atlassian/{version}` on all requests
- Use `PathEscape` for URL path segments, `QueryEscape` for query parameters

### Testing
- Unit tests: mock HTTP server via `httptest.NewServer`
- Acceptance tests: `TF_ACC=1`, run against demo Jira instance (`lbajsarowicz.atlassian.net`)
- All acceptance tests MUST have `CheckDestroy` function
- Test resource names MUST be randomized (`acctest.RandStringFromCharSet`)
- Mock servers MUST validate query parameters and request bodies
- Use `atomic.Int32` or `sync.Mutex` for thread-safe counters in handlers
- Shared test factories in `internal/testutil/testutil.go`

### Commits & PRs
- Conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`, `ci:`
- Atomic commits — one logical change per commit
- One PR per feature/resource family
- Codex code review must pass before merge
- Semantic versioning based on conventional commits

### File Organization
```
main.go                          # Provider entry point
internal/
  provider/provider.go           # Provider definition + schema
  atlassian/client.go            # HTTP client (auth, retry, rate limit)
  atlassian/pagination.go        # Pagination helpers (offset + cursor)
  jira/group_resource.go         # Resource: atlassian_jira_group
  jira/group_data_source.go      # Data source: atlassian_jira_group
  jira/group_resource_test.go    # Tests
  ...                            # One file per resource/data source
  confluence/space_resource.go   # Resource: atlassian_confluence_space
  testutil/testutil.go           # Shared test helpers
docs/
  api-reference/                 # Local API docs cache
  context/                       # ADRs, design decisions, constraints
  competition/                   # Existing provider analysis
```

## Build & Test

```bash
make build        # Build binary
make install      # Install to local plugin dir (for dev_overrides)
make test         # Unit tests
make testacc      # Acceptance tests (requires ATLASSIAN_* env vars)
make lint         # golangci-lint
```

## What NOT to do

- Do NOT use `andygrunwald/go-jira` or any third-party Jira client
- Do NOT support `username` anywhere — only `accountId`
- Do NOT ignore 404 in Read — always call `RemoveResource()`
- Do NOT use plain `Delete` — use `DeleteWithStatus` and tolerate 404
- Do NOT skip `ImportState` on any resource
- Do NOT use `TypeSet` for ordered collections (field options, screen fields) — use `TypeList`
- Do NOT hardcode custom field values as strings — they can be maps, arrays, or nested objects
- Do NOT assume API responses are instant — handle eventual consistency
- Do NOT trust POST response to contain all fields — Jira returns partial responses, preserve plan values
- Do NOT use `QueryEscape` for URL path segments — use `PathEscape`
- Do NOT merge PRs without Codex code review passing
