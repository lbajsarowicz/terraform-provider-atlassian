package confluence

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &spaceDataSource{}

// NewSpaceDataSource returns a new Confluence space data source.
func NewSpaceDataSource() datasource.DataSource {
	return &spaceDataSource{}
}

type spaceDataSource struct {
	client *atlassian.Client
}

type spaceDataSourceModel struct {
	Key         types.String `tfsdk:"key"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *spaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space"
}

func (d *spaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Confluence Cloud space by key.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Description: "The space key to look up (e.g. MYSPACE).",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The numeric ID of the space.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the space.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The plain-text description of the space.",
				Computed:    true,
			},
		},
	}
}

func (d *spaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *spaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config spaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := config.Key.ValueString()
	apiPath := fmt.Sprintf("/wiki/api/v2/spaces?keys=%s&limit=1", atlassian.QueryEscape(key))

	var listResp struct {
		Results []spaceV2Response `json:"results"`
	}
	if err := d.client.Get(ctx, apiPath, &listResp); err != nil {
		resp.Diagnostics.AddError("Error reading Confluence space", err.Error())
		return
	}

	if len(listResp.Results) == 0 {
		resp.Diagnostics.AddError(
			"Confluence space not found",
			fmt.Sprintf("No Confluence space found with key %q", key),
		)
		return
	}

	space := listResp.Results[0]
	config.ID = types.StringValue(space.ID.String())
	config.Key = types.StringValue(space.Key)
	config.Name = types.StringValue(space.Name)
	if space.Description != nil {
		config.Description = types.StringValue(space.Description.Value)
	} else {
		config.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
