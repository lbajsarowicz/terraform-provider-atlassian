package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource = &permissionSchemeGrantResource{}
)

// NewPermissionSchemeGrantResource returns a new permission scheme grant resource.
func NewPermissionSchemeGrantResource() resource.Resource {
	return &permissionSchemeGrantResource{}
}

type permissionSchemeGrantResource struct {
	client *atlassian.Client
}

type permissionSchemeGrantResourceModel struct {
	ID              types.String `tfsdk:"id"`
	SchemeID        types.String `tfsdk:"scheme_id"`
	Permission      types.String `tfsdk:"permission"`
	HolderType      types.String `tfsdk:"holder_type"`
	HolderParameter types.String `tfsdk:"holder_parameter"`
}

// permissionSchemeGrantAPIResponse represents the Jira permission scheme grant API response.
type permissionSchemeGrantAPIResponse struct {
	ID     int `json:"id"`
	Holder struct {
		Type      string `json:"type"`
		Parameter string `json:"parameter,omitempty"`
	} `json:"holder"`
	Permission string `json:"permission"`
}

// permissionSchemeGrantCreateRequest represents the POST request body for creating a grant.
type permissionSchemeGrantCreateRequest struct {
	Holder     grantHolder `json:"holder"`
	Permission string      `json:"permission"`
}

type grantHolder struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter,omitempty"`
}

func (r *permissionSchemeGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_permission_scheme_grant"
}

func (r *permissionSchemeGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a permission grant within a Jira Cloud permission scheme. Grants are immutable; any change forces recreation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the permission grant.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scheme_id": schema.StringAttribute{
				Description: "The ID of the permission scheme this grant belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permission": schema.StringAttribute{
				Description: "The permission to grant (e.g. BROWSE_PROJECTS, ADMINISTER_PROJECTS).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"holder_type": schema.StringAttribute{
				Description: "The type of the permission holder.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						"group", "projectRole", "user", "applicationRole",
						"reporter", "projectLead", "assignee", "anyone",
					),
				},
			},
			"holder_parameter": schema.StringAttribute{
				Description: "The parameter for the holder (group name, role ID, account ID). Not required for reporter, projectLead, assignee, anyone.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *permissionSchemeGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *permissionSchemeGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionSchemeGrantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	holder := grantHolder{
		Type: plan.HolderType.ValueString(),
	}
	if !plan.HolderParameter.IsNull() && !plan.HolderParameter.IsUnknown() {
		holder.Parameter = plan.HolderParameter.ValueString()
	}

	body := permissionSchemeGrantCreateRequest{
		Holder:     holder,
		Permission: plan.Permission.ValueString(),
	}

	var result permissionSchemeGrantAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s/permission", atlassian.QueryEscape(plan.SchemeID.ValueString()))
	err := r.client.Post(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating permission scheme grant", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	plan.Permission = types.StringValue(result.Permission)
	plan.HolderType = types.StringValue(result.Holder.Type)
	if result.Holder.Parameter != "" {
		plan.HolderParameter = types.StringValue(result.Holder.Parameter)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *permissionSchemeGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionSchemeGrantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result permissionSchemeGrantAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s/permission/%s",
		atlassian.QueryEscape(state.SchemeID.ValueString()),
		atlassian.QueryEscape(state.ID.ValueString()),
	)
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission scheme grant", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	state.Permission = types.StringValue(result.Permission)
	state.HolderType = types.StringValue(result.Holder.Type)
	if result.Holder.Parameter != "" {
		state.HolderParameter = types.StringValue(result.Holder.Parameter)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *permissionSchemeGrantResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Permission scheme grants are immutable. All attribute changes require replacing the resource.",
	)
}

func (r *permissionSchemeGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionSchemeGrantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/permissionscheme/%s/permission/%s",
		atlassian.QueryEscape(state.SchemeID.ValueString()),
		atlassian.QueryEscape(state.ID.ValueString()),
	)
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting permission scheme grant", err.Error())
		return
	}

	// 404 means the grant was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}
