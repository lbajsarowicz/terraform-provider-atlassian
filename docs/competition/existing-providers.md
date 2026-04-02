# Existing Terraform/OpenTofu Providers for Atlassian

## fourplusone/jira (most mature, abandoned)
- **GitHub:** https://github.com/fourplusone/terraform-provider-jira
- **Stars:** 183 | **Last commit:** July 2023 | **Version:** 0.1.16
- **Resources:** Comments, Components, Filters, Groups, Issues, Issue Types, Projects, Roles, Users, Webhooks
- **Missing:** Permission schemes, workflow schemes, screen schemes, field configs, notifications, Confluence
- **Broken:** `username` deprecated (#74,#83,#98), `go-jira` library bottleneck, race conditions (#73), custom fields hardcoded as strings (#93), avatar_id no read (#67)
- **Unmerged PRs:** Permission schemes (#104), issue type schemes (#25), accountId fix (#98)
- **Lesson:** Governance resources (schemes, permissions) are the gap. Operational resources (issues, comments) are not the value-add.

## surajrajput1024/atlassian (young)
- **GitHub:** https://github.com/surajrajput1024/terraform-provider-atlassian
- **Stars:** 4 | **Version:** v0.1.0
- **Resources:** project, permission_scheme, permission_grant, project_permission_scheme, project_role_actor, group, workflow_scheme_attachment
- **Assessment:** Too immature, single developer, no test suite visible

## atlassian/atlassian-operations (official, wrong scope)
- **GitHub:** https://github.com/atlassian/terraform-provider-atlassian-operations
- **Scope:** JSM operations + Compass only. Not Jira/Confluence configuration.

## chesshacker/confluence (minimal)
- **Resources:** confluence_content, confluence_attachment
- **Assessment:** Content management only, no space config

## DrFaust92/bitbucket (maintained, different product)
- **Stars:** ~169 | Covers Bitbucket Cloud. Not our scope.

## Our differentiation
We are the ONLY provider covering:
1. Jira scheme management (permission, workflow, screen, notification, field config)
2. Scheme-to-project associations
3. Custom field lifecycle (fields + contexts + options)
4. Confluence space configuration
5. Full import support + drift detection
6. Modern Plugin Framework (not legacy SDK v2)
7. Direct HTTP client with pagination, rate limiting, retries
