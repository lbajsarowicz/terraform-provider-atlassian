package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &groupResource{}
	_ resource.ResourceWithImportState = &groupResource{}
)

// NewGroupResource returns a new group resource.
func NewGroupResource() resource.Resource {
	return &groupResource{}
}

type groupResource struct {
	client *atlassian.Client
}

type groupResourceModel struct {
	GroupID types.String `tfsdk:"group_id"`
	Name    types.String `tfsdk:"name"`
}

// groupAPIResponse represents the Jira group API response shape.
type groupAPIResponse struct {
	GroupID string `json:"groupId"`
	Name    string `json:"name"`
	Self    string `json:"self,omitempty"`
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud group.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Description: "The ID of the group.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the group. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]string{
		"name": plan.Name.ValueString(),
	}

	var result groupAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/group", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating group", err.Error())
		return
	}

	plan.GroupID = types.StringValue(result.GroupID)
	plan.Name = types.StringValue(result.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result groupAPIResponse
	path := fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(state.GroupID.ValueString()))
	statusCode, err := r.client.GetWithStatus(ctx, path, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.GroupID = types.StringValue(result.GroupID)
	state.Name = types.StringValue(result.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *groupResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Jira groups cannot be updated. Name changes require replacing the resource.",
	)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := fmt.Sprintf("/rest/api/3/group?groupId=%s", atlassian.QueryEscape(state.GroupID.ValueString()))
	err := r.client.Delete(ctx, path)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting group", err.Error())
		return
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by group name — resolve to groupId
	groupName := req.ID

	var result groupAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/group?groupname=%s", atlassian.QueryEscape(groupName))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error importing group", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Group not found",
			fmt.Sprintf("No group found with name %q", groupName),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), result.GroupID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), result.Name)...)
}
