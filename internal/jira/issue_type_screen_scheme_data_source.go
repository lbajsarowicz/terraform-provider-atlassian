package jira

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &issueTypeScreenSchemeDataSource{}

// NewIssueTypeScreenSchemeDataSource returns a new issue type screen scheme data source.
func NewIssueTypeScreenSchemeDataSource() datasource.DataSource {
	return &issueTypeScreenSchemeDataSource{}
}

type issueTypeScreenSchemeDataSource struct {
	client *atlassian.Client
}

type issueTypeScreenSchemeDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
}

func (d *issueTypeScreenSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_screen_scheme"
}

func (d *issueTypeScreenSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud issue type screen scheme by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the issue type screen scheme to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the issue type screen scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the issue type screen scheme.",
				Computed:    true,
			},
		},
	}
}

func (d *issueTypeScreenSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *atlassian.Client, got: %T", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *issueTypeScreenSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config issueTypeScreenSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()

	allValues, err := d.client.GetAllPages(ctx, "/rest/api/3/issuetypescreenscheme")
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type screen schemes", err.Error())
		return
	}

	var found *issueTypeScreenSchemeAPIResponse
	for _, raw := range allValues {
		var scheme issueTypeScreenSchemeAPIResponse
		if err := json.Unmarshal(raw, &scheme); err != nil {
			resp.Diagnostics.AddError("Error parsing issue type screen scheme", err.Error())
			return
		}
		if scheme.Name == name {
			if found != nil {
				resp.Diagnostics.AddError(
					"Ambiguous issue type screen scheme name",
					fmt.Sprintf("More than one issue type screen scheme found with name %q", name),
				)
				return
			}
			schemeCopy := scheme
			found = &schemeCopy
		}
	}

	if found == nil {
		resp.Diagnostics.AddError(
			"Issue type screen scheme not found",
			fmt.Sprintf("No issue type screen scheme found with name %q", name),
		)
		return
	}

	config.ID = types.StringValue(found.ID)
	config.Description = types.StringValue(found.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
