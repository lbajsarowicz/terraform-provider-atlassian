package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &issueTypeResource{}
	_ resource.ResourceWithImportState = &issueTypeResource{}
)

// NewIssueTypeResource returns a new issue type resource.
func NewIssueTypeResource() resource.Resource {
	return &issueTypeResource{}
}

type issueTypeResource struct {
	client *atlassian.Client
}

type issueTypeResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Type           types.String `tfsdk:"type"`
	HierarchyLevel types.Int64  `tfsdk:"hierarchy_level"`
	AvatarID       types.Int64  `tfsdk:"avatar_id"`
}

// issueTypeAPIResponse represents the Jira issue type API response shape.
type issueTypeAPIResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int64  `json:"hierarchyLevel"`
	AvatarID       int64  `json:"avatarId"`
}

// issueTypeWriteRequest represents the Jira issue type create/update request body.
type issueTypeWriteRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	AvatarID    int64  `json:"avatarId,omitempty"`
}

func (r *issueTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type"
}

func (r *issueTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud issue type.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the issue type.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue type.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the issue type.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"type": schema.StringAttribute{
				Description: `The type of issue type. Must be "standard" or "subtask". Changing this forces recreation of the resource.`,
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("standard", "subtask"),
				},
			},
			"hierarchy_level": schema.Int64Attribute{
				Description: "The hierarchy level of the issue type, set by Jira based on the type.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"avatar_id": schema.Int64Attribute{
				Description: "The ID of the avatar for the issue type.",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

func (r *issueTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *issueTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan issueTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := issueTypeWriteRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Type:        plan.Type.ValueString(),
	}
	if !plan.AvatarID.IsNull() && !plan.AvatarID.IsUnknown() {
		body.AvatarID = plan.AvatarID.ValueInt64()
	}

	var result issueTypeAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/issuetype", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating issue type", err.Error())
		return
	}

	// Server-generated: ID and HierarchyLevel always come from response.
	plan.ID = types.StringValue(result.ID)
	plan.HierarchyLevel = types.Int64Value(result.HierarchyLevel)

	// AvatarID: use plan value when explicitly set, otherwise take server default.
	if plan.AvatarID.IsNull() || plan.AvatarID.IsUnknown() {
		plan.AvatarID = types.Int64Value(result.AvatarID)
	}

	// Name, Description, Type are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *issueTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state issueTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result issueTypeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(result.ID)
	state.Name = types.StringValue(result.Name)
	state.Description = types.StringValue(result.Description)
	state.HierarchyLevel = types.Int64Value(result.HierarchyLevel)
	state.AvatarID = types.Int64Value(result.AvatarID)

	// Map subtask bool to type string
	if result.Subtask {
		state.Type = types.StringValue("subtask")
	} else {
		state.Type = types.StringValue("standard")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *issueTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan issueTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state issueTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := issueTypeWriteRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Type:        plan.Type.ValueString(),
	}
	if !plan.AvatarID.IsNull() && !plan.AvatarID.IsUnknown() {
		body.AvatarID = plan.AvatarID.ValueInt64()
	}

	var result issueTypeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(state.ID.ValueString()))
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating issue type", err.Error())
		return
	}

	// Server-generated: HierarchyLevel comes from response.
	plan.HierarchyLevel = types.Int64Value(result.HierarchyLevel)

	// AvatarID: use plan value when explicitly set, otherwise take server value.
	if plan.AvatarID.IsNull() || plan.AvatarID.IsUnknown() {
		plan.AvatarID = types.Int64Value(result.AvatarID)
	}

	// ID is carried forward from state (unchanged on update).
	// Name, Description, Type are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *issueTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state issueTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/issuetype/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting issue type", err.Error())
		return
	}

	// 404 means the issue type was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *issueTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
