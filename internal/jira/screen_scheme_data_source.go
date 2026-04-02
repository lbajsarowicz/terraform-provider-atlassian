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

var _ datasource.DataSource = &screenSchemeDataSource{}

// NewScreenSchemeDataSource returns a new screen scheme data source.
func NewScreenSchemeDataSource() datasource.DataSource {
	return &screenSchemeDataSource{}
}

type screenSchemeDataSource struct {
	client *atlassian.Client
}

type screenSchemeDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Screens     types.Map    `tfsdk:"screens"` // operation -> screen_id (string)
}

func (d *screenSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_scheme"
}

func (d *screenSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud screen scheme by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the screen scheme to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the screen scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the screen scheme.",
				Computed:    true,
			},
			"screens": schema.MapAttribute{
				Description: `A map of issue operation names to screen IDs. Valid keys: "default", "create", "view", "edit".`,
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *screenSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *screenSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config screenSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	allValues, err := d.client.GetAllPages(ctx, "/rest/api/3/screenscheme")
	if err != nil {
		resp.Diagnostics.AddError("Error reading screen schemes", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, raw := range allValues {
		var scheme screenSchemeAPIResponse
		if err := json.Unmarshal(raw, &scheme); err != nil {
			resp.Diagnostics.AddError("Error parsing screen scheme response", err.Error())
			return
		}
		if scheme.Name == name {
			config.ID = types.StringValue(fmt.Sprintf("%d", scheme.ID))
			config.Description = types.StringValue(scheme.Description)

			screensStr := screensInt64ToStr(scheme.Screens)
			screensValue, diags := types.MapValueFrom(ctx, types.StringType, screensStr)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			config.Screens = screensValue

			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Screen scheme not found",
		fmt.Sprintf("No screen scheme found with name %q", name),
	)
}
