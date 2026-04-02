package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &workflowResource{}
	_ resource.ResourceWithImportState = &workflowResource{}
)

// NewWorkflowResource returns a new workflow resource.
func NewWorkflowResource() resource.Resource {
	return &workflowResource{}
}

type workflowResource struct {
	client *atlassian.Client
}

type workflowResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Statuses    types.List   `tfsdk:"statuses"`
}

// API request/response types

type workflowCreateRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Statuses    []workflowStatusRef `json:"statuses"`
	Transitions []interface{}       `json:"transitions"`
}

type workflowStatusRef struct {
	StatusReference string         `json:"statusReference"`
	Layout          workflowLayout `json:"layout"`
}

type workflowLayout struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type workflowCreateResponse struct {
	ID struct {
		EntityID string `json:"entityId"`
	} `json:"id"`
	Name string `json:"name"`
}

type workflowAPIItem struct {
	ID struct {
		EntityID string `json:"entityId"`
	} `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Statuses    []struct {
		ID              string `json:"id"`
		StatusReference string `json:"statusReference"`
	} `json:"statuses"`
}

type workflowSearchResponse struct {
	Values []workflowAPIItem `json:"values"`
}

func (r *workflowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow"
}

func (r *workflowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud workflow (structure only: name, description, statuses). Transitions are not managed in v1.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The entity ID (UUID) of the workflow.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "The description of the workflow. Changing this forces recreation of the resource.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"statuses": schema.ListAttribute{
				Description: "List of status reference UUIDs used by the workflow. Changing this forces recreation of the resource.",
				Required:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *workflowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *workflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workflowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract status IDs from the plan list
	var statusIDs []string
	resp.Diagnostics.Append(plan.Statuses.ElementsAs(ctx, &statusIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	statusRefs := make([]workflowStatusRef, len(statusIDs))
	for i, id := range statusIDs {
		statusRefs[i] = workflowStatusRef{
			StatusReference: id,
			Layout:          workflowLayout{X: 0.0, Y: 0.0},
		}
	}

	body := workflowCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Statuses:    statusRefs,
		Transitions: []interface{}{},
	}

	var result workflowCreateResponse
	err := r.client.Post(ctx, "/rest/api/3/workflow/create", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating workflow", err.Error())
		return
	}

	// Only take the ID from the response; preserve all other plan values.
	plan.ID = types.StringValue(result.ID.EntityID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/workflow/search?workflowName=%s", atlassian.QueryEscape(state.Name.ValueString()))

	var searchResp workflowSearchResponse
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &searchResp)
	if err != nil {
		resp.Diagnostics.AddError("Error reading workflow", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	// Find the workflow by entity ID
	var found *workflowAPIItem
	for i := range searchResp.Values {
		if searchResp.Values[i].ID.EntityID == state.ID.ValueString() {
			found = &searchResp.Values[i]
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(found.ID.EntityID)
	state.Name = types.StringValue(found.Name)
	state.Description = types.StringValue(found.Description)

	// Extract status IDs from the API response
	statusIDs := make([]string, len(found.Statuses))
	for i, s := range found.Statuses {
		statusIDs[i] = s.ID
	}

	statusList, diags := types.ListValueFrom(ctx, types.StringType, statusIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Statuses = statusList

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *workflowResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Jira workflows cannot be updated in v1. All fields are ForceNew.",
	)
}

func (r *workflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state workflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := "/rest/api/3/workflow?workflowId=" + atlassian.QueryEscape(state.ID.ValueString())
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting workflow", err.Error())
		return
	}

	// 404 means the workflow was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *workflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
