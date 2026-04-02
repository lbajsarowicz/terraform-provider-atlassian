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
	if os.Getenv("TF_ACC") != "1" {
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
