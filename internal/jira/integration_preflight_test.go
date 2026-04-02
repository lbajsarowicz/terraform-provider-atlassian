package jira_test

import (
	"context"
	"testing"

	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

// TestIntegration_preflight verifies API access and admin permissions before the suite runs.
// If this test fails, all other integration tests will also fail — check credentials.
func TestIntegration_preflight(t *testing.T) {
	testutil.SkipIfNoAcc(t)

	client, err := testutil.SweepClient()
	if err != nil {
		t.Fatalf("preflight: cannot create client: %s", err)
	}

	ctx := context.Background()

	// Verify we can list projects (requires project-level read access).
	var result interface{}
	statusCode, err := client.GetWithStatus(ctx, "/rest/api/3/project/search?maxResults=1", &result)
	if err != nil {
		t.Fatalf("preflight: API access failed (status %d): %s", statusCode, err)
	}

	// Verify we can list groups (requires admin-level access).
	var groupResult interface{}
	statusCode, err = client.GetWithStatus(ctx, "/rest/api/3/group/bulk?maxResults=1", &groupResult)
	if err != nil {
		t.Fatalf("preflight: admin access failed (status %d) — check that the API token belongs to a Jira admin: %s", statusCode, err)
	}
}
