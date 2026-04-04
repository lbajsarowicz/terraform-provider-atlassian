package jira

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &projectWorkflowSchemeResource{}
	_ resource.ResourceWithImportState = &projectWorkflowSchemeResource{}
)

// NewProjectWorkflowSchemeResource returns a new project workflow scheme association resource.
func NewProjectWorkflowSchemeResource() resource.Resource {
	return &projectWorkflowSchemeResource{}
}

type projectWorkflowSchemeResource struct {
	client *atlassian.Client
}

type projectWorkflowSchemeResourceModel struct {
	ProjectID        types.String `tfsdk:"project_id"`
	WorkflowSchemeID types.String `tfsdk:"workflow_scheme_id"`
}

// projectWorkflowSchemeListEntry represents a single entry in the
// GET /rest/api/3/workflowscheme/project response values array.
type projectWorkflowSchemeListEntry struct {
	WorkflowScheme workflowSchemeAPIResponse `json:"workflowScheme"`
}

func (r *projectWorkflowSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_workflow_scheme"
}

func (r *projectWorkflowSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Associates a Jira Cloud project with a workflow scheme.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Description: "The ID of the project. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"workflow_scheme_id": schema.StringAttribute{
				Description: "The ID of the workflow scheme to assign. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *projectWorkflowSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectWorkflowSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectWorkflowSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]string{
		"workflowSchemeId": plan.WorkflowSchemeID.ValueString(),
		"projectId":        plan.ProjectID.ValueString(),
	}

	err := r.client.Put(ctx, "/rest/api/3/workflowscheme/project", body, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error assigning workflow scheme to project", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectWorkflowSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectWorkflowSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()
	apiPath := fmt.Sprintf("/rest/api/3/workflowscheme/project?projectId=%s", atlassian.QueryEscape(projectID))

	// This endpoint returns at most one entry per project — no pagination needed.
	var page atlassian.PageResponse
	if err := r.client.Get(ctx, apiPath, &page); err != nil {
		resp.Diagnostics.AddError("Error reading project workflow scheme", err.Error())
		return
	}

	for _, raw := range page.Values {
		var entry projectWorkflowSchemeListEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			resp.Diagnostics.AddError("Error parsing project workflow scheme response", err.Error())
			return
		}
		schemeIDStr := fmt.Sprintf("%d", entry.WorkflowScheme.ID)
		if schemeIDStr == state.WorkflowSchemeID.ValueString() {
			state.WorkflowSchemeID = types.StringValue(schemeIDStr)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Association not found — remove from state.
	resp.State.RemoveResource(ctx)
}

func (r *projectWorkflowSchemeResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both fields have RequiresReplace, so Update is never called.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Changing project_id or workflow_scheme_id requires replacing the resource.",
	)
}

func (r *projectWorkflowSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectWorkflowSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The Jira API does not support removing a workflow scheme association.
	// On destroy, we perform a no-op (leave the project with its current scheme).
	// This matches the behaviour described in the task spec.
}

func (r *projectWorkflowSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID := req.ID

	apiPath := fmt.Sprintf("/rest/api/3/workflowscheme/project?projectId=%s", atlassian.QueryEscape(projectID))

	// This endpoint returns at most one entry per project — no pagination needed.
	var page atlassian.PageResponse
	if err := r.client.Get(ctx, apiPath, &page); err != nil {
		resp.Diagnostics.AddError("Error importing project workflow scheme", err.Error())
		return
	}

	if len(page.Values) == 0 {
		resp.Diagnostics.AddError(
			"Project workflow scheme not found",
			fmt.Sprintf("No workflow scheme association found for project ID %q", projectID),
		)
		return
	}

	var entry projectWorkflowSchemeListEntry
	if err := json.Unmarshal(page.Values[0], &entry); err != nil {
		resp.Diagnostics.AddError("Error parsing project workflow scheme response", err.Error())
		return
	}
	schemeIDStr := fmt.Sprintf("%d", entry.WorkflowScheme.ID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workflow_scheme_id"), schemeIDStr)...)
}
