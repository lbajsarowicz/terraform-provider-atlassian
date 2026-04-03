package jira

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &statusDataSource{}

// NewStatusDataSource returns a new status data source.
func NewStatusDataSource() datasource.DataSource {
	return &statusDataSource{}
}

type statusDataSource struct {
	client *atlassian.Client
}

type statusDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	StatusCategory types.String `tfsdk:"status_category"`
}

func (d *statusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_status"
}

func (d *statusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud status by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the status to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the status.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the status.",
				Computed:    true,
			},
			"status_category": schema.StringAttribute{
				Description: `The category of the status: "TODO", "IN_PROGRESS", or "DONE".`,
				Computed:    true,
			},
		},
	}
}

func (d *statusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *statusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config statusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	statuses, err := findAllStatuses(ctx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading statuses", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, s := range statuses {
		if s.Name == name {
			config.ID = types.StringValue(s.ID)
			config.Description = types.StringValue(s.Description)
			config.StatusCategory = types.StringValue(s.statusCategoryKey())

			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Status not found",
		fmt.Sprintf("No status found with name %q", name),
	)
}
