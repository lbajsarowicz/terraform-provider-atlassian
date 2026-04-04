# terraform-provider-atlassian

[![Terraform Registry](https://img.shields.io/badge/terraform-registry-blueviolet)](https://registry.terraform.io/providers/lbajsarowicz/atlassian/latest)
[![CI](https://github.com/lbajsarowicz/terraform-provider-atlassian/actions/workflows/ci.yml/badge.svg)](https://github.com/lbajsarowicz/terraform-provider-atlassian/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/lbajsarowicz/terraform-provider-atlassian)](https://go.dev/)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

Terraform/OpenTofu provider for managing [Atlassian Cloud](https://www.atlassian.com/cloud) (Jira + Confluence) configuration as infrastructure as code. Supports projects, permission schemes, workflows, issue types, custom fields, screens, roles, and Confluence spaces — with full import and drift detection.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0 or [OpenTofu](https://opentofu.org/docs/intro/install/) >= 1.6
- Go >= 1.25 (for development only)

## Installation

```hcl
terraform {
  required_providers {
    atlassian = {
      source  = "lbajsarowicz/atlassian"
      version = "~> 0.1"
    }
  }
}
```

> **Note:** Terraform and OpenTofu resolve `lbajsarowicz/atlassian` to their respective registries automatically. Lock files will differ between the two tools because the signing chains are different.

## Authentication

| Attribute | Environment Variable | Description |
|-----------|---------------------|-------------|
| `url`     | `ATLASSIAN_URL`     | Atlassian Cloud instance URL (e.g., `https://mysite.atlassian.net`) |
| `user`    | `ATLASSIAN_USER`    | Account email for API authentication |
| `token`   | `ATLASSIAN_TOKEN`   | [API token](https://id.atlassian.com/manage-profile/security/api-tokens) |

Provider configuration takes precedence over environment variables.

```hcl
provider "atlassian" {
  url   = "https://mysite.atlassian.net"
  user  = "admin@example.com"
  token = "your-api-token"
}
```

### Using 1Password CLI

```bash
op run --env-file=.env -- terraform plan
```

## Quick Start

```hcl
resource "atlassian_jira_project" "my_project" {
  key              = "MYPROJ"
  name             = "My Project"
  project_type_key = "software"
  lead_account_id  = "5a1234abc"
}

data "atlassian_jira_project" "existing" {
  key = "EXIST"
}
```

## Resources

| Resource | Description |
|----------|-------------|
| `atlassian_jira_group` | Jira group |
| `atlassian_jira_project` | Jira project |
| `atlassian_jira_permission_scheme` | Permission scheme |
| `atlassian_jira_permission_scheme_grant` | Permission scheme grant |
| `atlassian_jira_project_permission_scheme` | Project ↔ permission scheme association |
| `atlassian_jira_issue_type` | Issue type |
| `atlassian_jira_issue_type_scheme` | Issue type scheme |
| `atlassian_jira_project_issue_type_scheme` | Project ↔ issue type scheme association |
| `atlassian_jira_project_role` | Project role |
| `atlassian_jira_project_role_actor` | Project role actor (user/group) |
| `atlassian_jira_custom_field` | Custom field |
| `atlassian_jira_status` | Workflow status |
| `atlassian_jira_workflow` | Workflow |
| `atlassian_jira_workflow_scheme` | Workflow scheme |
| `atlassian_jira_project_workflow_scheme` | Project ↔ workflow scheme association |
| `atlassian_jira_screen` | Screen |
| `atlassian_jira_screen_tab` | Screen tab |
| `atlassian_jira_screen_tab_field` | Screen tab field |
| `atlassian_jira_screen_scheme` | Screen scheme |
| `atlassian_jira_issue_type_screen_scheme` | Issue type screen scheme |
| `atlassian_jira_project_issue_type_screen_scheme` | Project ↔ issue type screen scheme association |
| `atlassian_confluence_space` | Confluence space |
| `atlassian_confluence_space_permission` | Confluence space permission |

## Data Sources

| Data Source | Description |
|-------------|-------------|
| `atlassian_jira_group` | Look up a Jira group by name |
| `atlassian_jira_project` | Look up a Jira project by key |
| `atlassian_jira_permission_scheme` | Look up a permission scheme by name |
| `atlassian_jira_issue_type` | Look up an issue type by name |
| `atlassian_jira_issue_type_scheme` | Look up an issue type scheme by name |
| `atlassian_jira_project_role` | Look up a project role by name |
| `atlassian_jira_custom_field` | Look up a custom field by name |
| `atlassian_jira_status` | Look up a workflow status by name |
| `atlassian_jira_workflow` | Look up a workflow by name |
| `atlassian_jira_workflow_scheme` | Look up a workflow scheme by name |
| `atlassian_jira_screen` | Look up a screen by name |
| `atlassian_jira_screen_scheme` | Look up a screen scheme by name |
| `atlassian_jira_issue_type_screen_scheme` | Look up an issue type screen scheme by name |
| `atlassian_confluence_space` | Look up a Confluence space by key |

## Development

### Build

```bash
make build
```

### Install locally (dev_overrides)

```bash
make install
```

Then add to `~/.terraformrc` or `~/.tofurc`:

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/lbajsarowicz/atlassian" = "<output of: make install>"
  }
  direct {}
}
```

### Run tests

```bash
make test              # Unit tests
make testacc           # Acceptance tests (requires ATLASSIAN_* env vars)
make testintegration   # Integration tests against real Jira
make lint              # Lint
```

### Generate documentation

```bash
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.24.0
tfplugindocs generate
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes with tests
4. Use [conventional commits](https://www.conventionalcommits.org/) — PR titles are enforced (`feat:`, `fix:`, `docs:`, etc.)
5. Run `make test` and `make lint`
6. Open a pull request

## Sponsor

If you find this provider useful, consider [sponsoring my work](https://github.com/sponsors/lbajsarowicz) so I can continue maintaining and improving it.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE) for details.
