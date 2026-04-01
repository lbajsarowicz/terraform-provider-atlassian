package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/jira"
)

var _ provider.Provider = &AtlassianProvider{}

type AtlassianProvider struct {
	version string
}

type AtlassianProviderModel struct {
	URL   types.String `tfsdk:"url"`
	User  types.String `tfsdk:"user"`
	Token types.String `tfsdk:"token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &AtlassianProvider{
			version: version,
		}
	}
}

func (p *AtlassianProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "atlassian"
	resp.Version = p.version
}

func (p *AtlassianProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Atlassian Cloud (Jira + Confluence) resources.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "Atlassian Cloud instance URL (e.g. https://mysite.atlassian.net). Falls back to ATLASSIAN_URL env var.",
				Optional:    true,
			},
			"user": schema.StringAttribute{
				Description: "Atlassian account email for API authentication. Falls back to ATLASSIAN_USER env var.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "Atlassian API token for authentication. Falls back to ATLASSIAN_TOKEN env var.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *AtlassianProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config AtlassianProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Pass values to client - empty string triggers env var fallback in NewClient
	url := ""
	if !config.URL.IsNull() && !config.URL.IsUnknown() {
		url = config.URL.ValueString()
	}
	user := ""
	if !config.User.IsNull() && !config.User.IsUnknown() {
		user = config.User.ValueString()
	}
	token := ""
	if !config.Token.IsNull() && !config.Token.IsUnknown() {
		token = config.Token.ValueString()
	}

	client, err := atlassian.NewClient(atlassian.ClientConfig{
		URL:     url,
		User:    user,
		Token:   token,
		Version: p.version,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Atlassian client", err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *AtlassianProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		jira.NewProjectResource,
	}
}

func (p *AtlassianProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
