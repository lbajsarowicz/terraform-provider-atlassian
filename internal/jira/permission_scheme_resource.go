package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &permissionSchemeResource{}
	_ resource.ResourceWithImportState = &permissionSchemeResource{}
)

// NewPermissionSchemeResource returns a new permission scheme resource.
func NewPermissionSchemeResource() resource.Resource {
	return &permissionSchemeResource{}
}

type permissionSchemeResource struct {
	client *atlassian.Client
}

type permissionSchemeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// permissionSchemeAPIResponse represents the Jira permission scheme API response shape.
type permissionSchemeAPIResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// permissionSchemeCreateRequest represents the POST/PUT request body.
type permissionSchemeCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (r *permissionSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_permission_scheme"
}

func (r *permissionSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud permission scheme.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the permission scheme.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the permission scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the permission scheme.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
		},
	}
}

func (r *permissionSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *permissionSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := permissionSchemeCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	var result permissionSchemeAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/permissionscheme", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating permission scheme", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	plan.Name = types.StringValue(result.Name)
	plan.Description = types.StringValue(result.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *permissionSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result permissionSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s", atlassian.QueryEscape(state.ID.ValueString()))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission scheme", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	state.Name = types.StringValue(result.Name)
	state.Description = types.StringValue(result.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *permissionSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan permissionSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := permissionSchemeCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	var result permissionSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s", atlassian.QueryEscape(plan.ID.ValueString()))
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating permission scheme", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	plan.Name = types.StringValue(result.Name)
	plan.Description = types.StringValue(result.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *permissionSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s", atlassian.QueryEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting permission scheme", err.Error())
		return
	}

	// 404 means the scheme was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *permissionSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	schemeID := req.ID

	var result permissionSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s", atlassian.QueryEscape(schemeID))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error importing permission scheme", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Permission scheme not found",
			fmt.Sprintf("No permission scheme found with ID %q", schemeID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), fmt.Sprintf("%d", result.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), result.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("description"), result.Description)...)
}
