package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/provider"
)

func TestProviderSchema(t *testing.T) {
	t.Parallel()

	resp, err := providerserver.NewProtocol6WithError(provider.New("test")())()
	if err != nil {
		t.Fatalf("failed to create provider server: %s", err)
	}

	schemaResp, err := resp.GetProviderSchema(t.Context(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("failed to get provider schema: %s", err)
	}

	if schemaResp.Provider == nil {
		t.Fatal("provider schema is nil")
	}

	if len(schemaResp.Provider.Block.Attributes) != 3 {
		t.Fatalf("expected 3 provider attributes, got %d", len(schemaResp.Provider.Block.Attributes))
	}

	expectedAttrs := []string{"url", "user", "token"}
	attrMap := make(map[string]bool)
	for _, attr := range schemaResp.Provider.Block.Attributes {
		attrMap[attr.Name] = true
	}

	for _, name := range expectedAttrs {
		if !attrMap[name] {
			t.Errorf("expected provider attribute %q not found", name)
		}
	}
}
