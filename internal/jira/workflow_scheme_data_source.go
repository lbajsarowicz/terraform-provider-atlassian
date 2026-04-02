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

var _ datasource.DataSource = &workflowSchemeDataSource{}

// NewWorkflowSchemeDataSource returns a new workflow scheme data source.
func NewWorkflowSchemeDataSource() datasource.DataSource {
	return &workflowSchemeDataSource{}
}

type workflowSchemeDataSource struct {
	client *atlassian.Client
}

type workflowSchemeDataSourceModel struct {
	Name              types.String `tfsdk:"name"`
	ID                types.String `tfsdk:"id"`
	Description       types.String `tfsdk:"description"`
	DefaultWorkflow   types.String `tfsdk:"default_workflow"`
	IssueTypeMappings types.Map    `tfsdk:"issue_type_mappings"`
}

func (d *workflowSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow_scheme"
}

func (d *workflowSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud workflow scheme by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the workflow scheme to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the workflow scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the workflow scheme.",
				Computed:    true,
			},
			"default_workflow": schema.StringAttribute{
				Description: "The name of the default workflow for the scheme.",
				Computed:    true,
			},
			"issue_type_mappings": schema.MapAttribute{
				Description: "A map of issue type IDs to workflow names.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *workflowSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *workflowSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config workflowSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// GET /rest/api/3/workflowscheme returns a paginated list.
	allValues, err := d.client.GetAllPages(ctx, "/rest/api/3/workflowscheme")
	if err != nil {
		resp.Diagnostics.AddError("Error reading workflow schemes", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, raw := range allValues {
		var scheme workflowSchemeAPIResponse
		if err := json.Unmarshal(raw, &scheme); err != nil {
			resp.Diagnostics.AddError("Error parsing workflow scheme response", err.Error())
			return
		}
		if scheme.Name == name {
			config.ID = types.StringValue(fmt.Sprintf("%d", scheme.ID))
			config.Description = types.StringValue(scheme.Description)
			config.DefaultWorkflow = types.StringValue(scheme.DefaultWorkflow)

			mappingsValue, diags := types.MapValueFrom(ctx, types.StringType, scheme.IssueTypeMappings)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			config.IssueTypeMappings = mappingsValue

			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Workflow scheme not found",
		fmt.Sprintf("No workflow scheme found with name %q", name),
	)
}
