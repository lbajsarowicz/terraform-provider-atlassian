package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &projectPermissionSchemeResource{}
	_ resource.ResourceWithImportState = &projectPermissionSchemeResource{}
)

// NewProjectPermissionSchemeResource returns a new project permission scheme association resource.
func NewProjectPermissionSchemeResource() resource.Resource {
	return &projectPermissionSchemeResource{}
}

type projectPermissionSchemeResource struct {
	client *atlassian.Client
}

type projectPermissionSchemeResourceModel struct {
	ProjectKey types.String `tfsdk:"project_key"`
	SchemeID   types.String `tfsdk:"scheme_id"`
}

// projectPermissionSchemeAPIResponse represents the GET response for a project's permission scheme.
type projectPermissionSchemeAPIResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (r *projectPermissionSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_permission_scheme"
}

func (r *projectPermissionSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Assigns a permission scheme to a Jira Cloud project.",
		Attributes: map[string]schema.Attribute{
			"project_key": schema.StringAttribute{
				Description: "The key of the project. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scheme_id": schema.StringAttribute{
				Description: "The ID of the permission scheme to assign to the project.",
				Required:    true,
			},
		},
	}
}

func (r *projectPermissionSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectPermissionSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectPermissionSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.assignScheme(ctx, plan.ProjectKey.ValueString(), plan.SchemeID.ValueString())...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectPermissionSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectPermissionSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result projectPermissionSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s/permissionscheme", atlassian.QueryEscape(state.ProjectKey.ValueString()))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project permission scheme", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.SchemeID = types.StringValue(fmt.Sprintf("%d", result.ID))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectPermissionSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectPermissionSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.assignScheme(ctx, plan.ProjectKey.ValueString(), plan.SchemeID.ValueString())...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectPermissionSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectPermissionSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reset to default permission scheme by looking it up.
	var listResult permissionSchemeListResponse
	_, err := r.client.GetWithStatus(ctx, "/rest/api/3/permissionscheme", &listResult)
	if err != nil {
		resp.Diagnostics.AddError("Error listing permission schemes", err.Error())
		return
	}

	defaultSchemeID := "0"
	for _, scheme := range listResult.PermissionSchemes {
		if scheme.Name == "Default Permission Scheme" {
			defaultSchemeID = fmt.Sprintf("%d", scheme.ID)
			break
		}
	}

	resp.Diagnostics.Append(r.assignScheme(ctx, state.ProjectKey.ValueString(), defaultSchemeID)...)
}

func (r *projectPermissionSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectKey := req.ID

	var result projectPermissionSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s/permissionscheme", atlassian.QueryEscape(projectKey))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error importing project permission scheme", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Project not found",
			fmt.Sprintf("No project found with key %q", projectKey),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_key"), projectKey)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scheme_id"), fmt.Sprintf("%d", result.ID))...)
}

// assignScheme assigns a permission scheme to a project via PUT.
func (r *projectPermissionSchemeResource) assignScheme(ctx context.Context, projectKey, schemeID string) diag.Diagnostics {
	var diags diag.Diagnostics

	body := map[string]string{
		"id": schemeID,
	}

	var result projectPermissionSchemeAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s/permissionscheme", atlassian.QueryEscape(projectKey))
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		diags.AddError("Error assigning permission scheme to project", err.Error())
	}

	return diags
}
