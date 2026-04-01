# terraform-provider-atlassian

Custom OpenTofu/Terraform provider for managing Atlassian Cloud (Jira + Confluence) configuration as code.

## Status

This provider is under active development. Resources and data sources will be added incrementally.

## Installation

### Development (dev_overrides)

During development, use `dev_overrides` in your OpenTofu/Terraform CLI configuration:

```bash
make install
```

Then add to `~/.terraformrc` or `~/.tofurc`:

```hcl
provider_installation {
  dev_overrides {
    "registry.opentofu.org/lbajsarowicz/atlassian" = "~/.terraform.d/plugins/registry.opentofu.org/lbajsarowicz/atlassian/0.1.0/<OS>_<ARCH>"
  }
  direct {}
}
```

Replace `<OS>_<ARCH>` with your platform (e.g., `darwin_arm64`, `linux_amd64`).

## Usage

```hcl
terraform {
  required_providers {
    atlassian = {
      source  = "registry.opentofu.org/lbajsarowicz/atlassian"
      version = "~> 0.1"
    }
  }
}

provider "atlassian" {
  url   = "https://mysite.atlassian.net"
  user  = "admin@example.com"
  token = "your-api-token"
}
```

## Authentication

The provider supports three authentication attributes, each of which can be set via provider configuration or environment variables:

| Attribute | Environment Variable | Description |
|-----------|---------------------|-------------|
| `url`     | `ATLASSIAN_URL`     | Atlassian Cloud instance URL |
| `user`    | `ATLASSIAN_USER`    | Account email for API auth |
| `token`   | `ATLASSIAN_TOKEN`   | API token |

Explicit provider configuration takes precedence over environment variables.

### Using 1Password CLI

For secure credential management with 1Password:

```bash
export ATLASSIAN_URL="$(op read 'op://terraform-provider-atlassian/atlassian-api-token/url')"
export ATLASSIAN_USER="$(op read 'op://terraform-provider-atlassian/atlassian-api-token/username')"
export ATLASSIAN_TOKEN="$(op read 'op://terraform-provider-atlassian/atlassian-api-token/api-token')"
```

## Development

### Prerequisites

- Go 1.23+
- OpenTofu or Terraform CLI

### Build

```bash
make build
```

### Install locally

```bash
make install
```

### Run tests

```bash
# Unit tests
make test

# Acceptance tests (requires ATLASSIAN_* env vars)
make testacc

# Lint
make lint
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes with tests
4. Use conventional commits (`feat:`, `fix:`, `docs:`, etc.)
5. Run `make test` and `make lint`
6. Open a pull request

## License

GPL-3.0-or-later. See [LICENSE](LICENSE) for details.
