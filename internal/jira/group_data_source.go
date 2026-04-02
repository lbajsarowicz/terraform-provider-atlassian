package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &groupDataSource{}

// NewGroupDataSource returns a new group data source.
func NewGroupDataSource() datasource.DataSource {
	return &groupDataSource{}
}

type groupDataSource struct {
	client *atlassian.Client
}

type groupDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	GroupID types.String `tfsdk:"group_id"`
}

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud group by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the group to look up.",
				Required:    true,
			},
			"group_id": schema.StringAttribute{
				Description: "The ID of the group.",
				Computed:    true,
			},
		},
	}
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config groupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result groupAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/group?groupname=%s", atlassian.QueryEscape(config.Name.ValueString()))
	statusCode, err := d.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Group not found",
			fmt.Sprintf("No group found with name %q", config.Name.ValueString()),
		)
		return
	}

	config.GroupID = types.StringValue(result.GroupID)
	config.Name = types.StringValue(result.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
