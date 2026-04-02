package jira

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &projectRoleDataSource{}

// NewProjectRoleDataSource returns a new project role data source.
func NewProjectRoleDataSource() datasource.DataSource {
	return &projectRoleDataSource{}
}

type projectRoleDataSource struct {
	client *atlassian.Client
}

type projectRoleDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *projectRoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_role"
}

func (d *projectRoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud project role by name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the project role.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the project role to look up.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the project role.",
				Computed:    true,
			},
		},
	}
}

func (d *projectRoleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *projectRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectRoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The Jira API has no "get role by name" endpoint. List all roles and find by name.
	var roles []projectRoleAPIResponse
	err := d.client.Get(ctx, "/rest/api/3/role", &roles)
	if err != nil {
		resp.Diagnostics.AddError("Error listing project roles", err.Error())
		return
	}

	targetName := config.Name.ValueString()
	for _, role := range roles {
		if role.Name == targetName {
			config.ID = types.StringValue(fmt.Sprintf("%d", role.ID))
			config.Name = types.StringValue(role.Name)
			config.Description = types.StringValue(role.Description)

			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Project role not found",
		fmt.Sprintf("No project role found with name %q", targetName),
	)
}
