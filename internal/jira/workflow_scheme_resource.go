package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &workflowSchemeResource{}
	_ resource.ResourceWithImportState = &workflowSchemeResource{}
)

// NewWorkflowSchemeResource returns a new workflow scheme resource.
func NewWorkflowSchemeResource() resource.Resource {
	return &workflowSchemeResource{}
}

type workflowSchemeResource struct {
	client *atlassian.Client
}

type workflowSchemeResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	DefaultWorkflow   types.String `tfsdk:"default_workflow"`
	IssueTypeMappings types.Map    `tfsdk:"issue_type_mappings"`
}

// workflowSchemeAPIResponse represents the Jira workflow scheme API response shape.
type workflowSchemeAPIResponse struct {
	ID                int64             `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	DefaultWorkflow   string            `json:"defaultWorkflow"`
	IssueTypeMappings map[string]string `json:"issueTypeMappings"`
	Draft             bool              `json:"draft"`
}

// workflowSchemeCreateRequest represents the Jira workflow scheme create/update request body.
type workflowSchemeCreateRequest struct {
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	DefaultWorkflow   string            `json:"defaultWorkflow,omitempty"`
	IssueTypeMappings map[string]string `json:"issueTypeMappings,omitempty"`
}

func (r *workflowSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow_scheme"
}

func (r *workflowSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud workflow scheme.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the workflow scheme.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the workflow scheme.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"default_workflow": schema.StringAttribute{
				Description: "The name of the default workflow for the scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"issue_type_mappings": schema.MapAttribute{
				Description: "A map of issue type IDs to workflow names.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *workflowSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *atlassian.Client, got: %T", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *workflowSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workflowSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := workflowSchemeCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	if !plan.DefaultWorkflow.IsNull() && !plan.DefaultWorkflow.IsUnknown() {
		body.DefaultWorkflow = plan.DefaultWorkflow.ValueString()
	}

	if !plan.IssueTypeMappings.IsNull() && !plan.IssueTypeMappings.IsUnknown() {
		mappings := make(map[string]string)
		resp.Diagnostics.Append(plan.IssueTypeMappings.ElementsAs(ctx, &mappings, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body.IssueTypeMappings = mappings
	}

	var result workflowSchemeAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/workflowscheme", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating workflow scheme", err.Error())
		return
	}

	// Only the ID is definitively from the server; preserve plan values but
	// resolve any Unknown computed fields using the server response.
	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))

	// Resolve Unknown computed values from the server response.
	if plan.DefaultWorkflow.IsUnknown() {
		plan.DefaultWorkflow = types.StringValue(result.DefaultWorkflow)
	}

	if plan.IssueTypeMappings.IsUnknown() {
		mappingsValue, diags := types.MapValueFrom(ctx, types.StringType, result.IssueTypeMappings)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.IssueTypeMappings = mappingsValue
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workflowSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workflowSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result workflowSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/workflowscheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading workflow scheme", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	state.Name = types.StringValue(result.Name)
	state.Description = types.StringValue(result.Description)
	state.DefaultWorkflow = types.StringValue(result.DefaultWorkflow)

	mappingsValue, diags := types.MapValueFrom(ctx, types.StringType, result.IssueTypeMappings)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.IssueTypeMappings = mappingsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *workflowSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan workflowSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state workflowSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := workflowSchemeCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	if !plan.DefaultWorkflow.IsNull() && !plan.DefaultWorkflow.IsUnknown() {
		body.DefaultWorkflow = plan.DefaultWorkflow.ValueString()
	}

	if !plan.IssueTypeMappings.IsNull() && !plan.IssueTypeMappings.IsUnknown() {
		mappings := make(map[string]string)
		resp.Diagnostics.Append(plan.IssueTypeMappings.ElementsAs(ctx, &mappings, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body.IssueTypeMappings = mappings
	}

	var result workflowSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/workflowscheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating workflow scheme", err.Error())
		return
	}

	// ID carries forward from state (unchanged on update); preserve all plan values
	// and resolve any Unknown computed fields using the server response.
	plan.ID = state.ID

	if plan.DefaultWorkflow.IsUnknown() {
		plan.DefaultWorkflow = types.StringValue(result.DefaultWorkflow)
	}

	if plan.IssueTypeMappings.IsUnknown() {
		mappingsValue, diags := types.MapValueFrom(ctx, types.StringType, result.IssueTypeMappings)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.IssueTypeMappings = mappingsValue
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workflowSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state workflowSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/workflowscheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting workflow scheme", err.Error())
		return
	}

	// 404 means the workflow scheme was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *workflowSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
