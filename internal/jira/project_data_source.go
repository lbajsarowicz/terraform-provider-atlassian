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

var _ datasource.DataSource = &projectDataSource{}

// NewProjectDataSource returns a new project data source.
func NewProjectDataSource() datasource.DataSource {
	return &projectDataSource{}
}

type projectDataSource struct {
	client *atlassian.Client
}

type projectDataSourceModel struct {
	Key            types.String `tfsdk:"key"`
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ProjectTypeKey types.String `tfsdk:"project_type_key"`
	LeadAccountID  types.String `tfsdk:"lead_account_id"`
}

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud project by key.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Description: "The project key to look up (e.g. \"PROJ\").",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the project.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the project.",
				Computed:    true,
			},
			"project_type_key": schema.StringAttribute{
				Description: "The project type key (e.g. software, service_desk, business).",
				Computed:    true,
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the project lead.",
				Computed:    true,
			},
		},
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result jiraProjectAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(config.Key.ValueString()))
	statusCode, err := d.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Project not found",
			fmt.Sprintf("No project found with key %q", config.Key.ValueString()),
		)
		return
	}

	config.ID = types.StringValue(result.ID)
	config.Key = types.StringValue(result.Key)
	config.Name = types.StringValue(result.Name)
	config.ProjectTypeKey = types.StringValue(result.ProjectTypeKey)
	config.LeadAccountID = types.StringValue(result.Lead.AccountID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
