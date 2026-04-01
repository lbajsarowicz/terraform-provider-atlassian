package testutil

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/provider"
)

// ProtoV6ProviderFactories returns provider factories for acceptance testing.
var ProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"atlassian": providerserver.NewProtocol6WithError(provider.New("test")()),
}
