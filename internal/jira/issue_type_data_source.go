package jira

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &issueTypeDataSource{}

// NewIssueTypeDataSource returns a new issue type data source.
func NewIssueTypeDataSource() datasource.DataSource {
	return &issueTypeDataSource{}
}

type issueTypeDataSource struct {
	client *atlassian.Client
}

type issueTypeDataSourceModel struct {
	Name           types.String `tfsdk:"name"`
	ID             types.String `tfsdk:"id"`
	Description    types.String `tfsdk:"description"`
	Type           types.String `tfsdk:"type"`
	HierarchyLevel types.Int64  `tfsdk:"hierarchy_level"`
	AvatarID       types.Int64  `tfsdk:"avatar_id"`
}

func (d *issueTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type"
}

func (d *issueTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud issue type by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the issue type to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the issue type.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the issue type.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: `The type of issue type: "standard" or "subtask".`,
				Computed:    true,
			},
			"hierarchy_level": schema.Int64Attribute{
				Description: "The hierarchy level of the issue type.",
				Computed:    true,
			},
			"avatar_id": schema.Int64Attribute{
				Description: "The ID of the avatar for the issue type.",
				Computed:    true,
			},
		},
	}
}

func (d *issueTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *issueTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config issueTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all issue types and find by name
	var issueTypes []issueTypeAPIResponse
	err := d.client.Get(ctx, "/rest/api/3/issuetype", &issueTypes)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue types", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, it := range issueTypes {
		if it.Name == name {
			config.ID = types.StringValue(it.ID)
			config.Description = types.StringValue(it.Description)
			config.HierarchyLevel = types.Int64Value(it.HierarchyLevel)
			config.AvatarID = types.Int64Value(it.AvatarID)

			if it.Subtask {
				config.Type = types.StringValue("subtask")
			} else {
				config.Type = types.StringValue("standard")
			}

			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Issue type not found",
		fmt.Sprintf("No issue type found with name %q", name),
	)
}
