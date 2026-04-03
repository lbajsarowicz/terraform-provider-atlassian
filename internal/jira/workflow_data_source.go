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

var _ datasource.DataSource = &workflowDataSource{}

// NewWorkflowDataSource returns a new workflow data source.
func NewWorkflowDataSource() datasource.DataSource {
	return &workflowDataSource{}
}

type workflowDataSource struct {
	client *atlassian.Client
}

type workflowDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Statuses    types.List   `tfsdk:"statuses"`
}

func (d *workflowDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow"
}

func (d *workflowDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud workflow by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the workflow to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The entity ID (UUID) of the workflow.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the workflow.",
				Computed:    true,
			},
			"statuses": schema.ListAttribute{
				Description: "List of status IDs used by the workflow.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *workflowDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *workflowDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config workflowDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/workflow/search?workflowName=%s&expand=statuses", atlassian.QueryEscape(config.Name.ValueString()))

	var searchResp workflowSearchResponse
	statusCode, err := d.client.GetWithStatus(ctx, apiPath, &searchResp)
	if err != nil {
		resp.Diagnostics.AddError("Error reading workflow", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Workflow not found",
			fmt.Sprintf("No workflow found with name %q", config.Name.ValueString()),
		)
		return
	}

	// Find the workflow by name in the response
	name := config.Name.ValueString()
	for _, wf := range searchResp.Values {
		if wf.ID.Name == name {
			config.ID = types.StringValue(wf.ID.EntityID)
			config.Description = types.StringValue(wf.Description)

			// Use numeric status ID to be consistent with the resource's create path.
			statusRefs := make([]string, len(wf.Statuses))
			for i, s := range wf.Statuses {
				statusRefs[i] = s.ID
			}

			statusList, diags := types.ListValueFrom(ctx, types.StringType, statusRefs)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}

			config.Statuses = statusList

			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Workflow not found",
		fmt.Sprintf("No workflow found with name %q", name),
	)
}
