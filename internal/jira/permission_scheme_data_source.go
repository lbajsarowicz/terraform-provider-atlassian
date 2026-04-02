package jira

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &permissionSchemeDataSource{}

// NewPermissionSchemeDataSource returns a new permission scheme data source.
func NewPermissionSchemeDataSource() datasource.DataSource {
	return &permissionSchemeDataSource{}
}

type permissionSchemeDataSource struct {
	client *atlassian.Client
}

type permissionSchemeDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// permissionSchemeListResponse represents the list endpoint response.
type permissionSchemeListResponse struct {
	PermissionSchemes []permissionSchemeAPIResponse `json:"permissionSchemes"`
}

func (d *permissionSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_permission_scheme"
}

func (d *permissionSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud permission scheme by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the permission scheme to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the permission scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the permission scheme.",
				Computed:    true,
			},
		},
	}
}

func (d *permissionSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *permissionSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config permissionSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result permissionSchemeListResponse
	_, err := d.client.GetWithStatus(ctx, "/rest/api/3/permissionscheme", &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission schemes", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, scheme := range result.PermissionSchemes {
		if scheme.Name == name {
			config.ID = types.StringValue(fmt.Sprintf("%d", scheme.ID))
			config.Name = types.StringValue(scheme.Name)
			config.Description = types.StringValue(scheme.Description)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Permission scheme not found",
		fmt.Sprintf("No permission scheme found with name %q", name),
	)
}
