package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &projectResource{}
	_ resource.ResourceWithImportState = &projectResource{}
)

// NewProjectResource returns a new project resource.
func NewProjectResource() resource.Resource {
	return &projectResource{}
}

type projectResource struct {
	client *atlassian.Client
}

type projectResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	ProjectTypeKey types.String `tfsdk:"project_type_key"`
	LeadAccountID  types.String `tfsdk:"lead_account_id"`
	AssigneeType   types.String `tfsdk:"assignee_type"`
}

// jiraProjectAPIResponse represents the Jira project API GET response shape.
type jiraProjectAPIResponse struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ProjectTypeKey string `json:"projectTypeKey"`
	Lead           struct {
		AccountID string `json:"accountId"`
	} `json:"lead"`
	AssigneeType string `json:"assigneeType"`
}

// jiraProjectCreateRequest represents the POST/PUT request body for creating/updating a project.
type jiraProjectCreateRequest struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ProjectTypeKey string `json:"projectTypeKey"`
	LeadAccountID  string `json:"leadAccountId"`
	AssigneeType   string `json:"assigneeType"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the project.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Description: "The project key (e.g. \"PROJ\"). Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the project.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the project.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"project_type_key": schema.StringAttribute{
				Description: "The project type key. One of: software, service_desk, business. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("software", "service_desk", "business"),
				},
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the project lead.",
				Required:    true,
			},
			"assignee_type": schema.StringAttribute{
				Description: "The default assignee type. One of: PROJECT_LEAD, UNASSIGNED.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("UNASSIGNED"),
				Validators: []validator.String{
					stringvalidator.OneOf("PROJECT_LEAD", "UNASSIGNED"),
				},
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := jiraProjectCreateRequest{
		Key:            plan.Key.ValueString(),
		Name:           plan.Name.ValueString(),
		Description:    plan.Description.ValueString(),
		ProjectTypeKey: plan.ProjectTypeKey.ValueString(),
		LeadAccountID:  plan.LeadAccountID.ValueString(),
		AssigneeType:   plan.AssigneeType.ValueString(),
	}

	var result jiraProjectAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/project", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating project", err.Error())
		return
	}

	// Jira POST /rest/api/3/project returns only {self, id, key} — not the full
	// project object. Use the response for id and key; keep plan values for
	// name, description, project_type_key, lead_account_id, and assignee_type.
	plan.ID = types.StringValue(result.ID)
	plan.Key = types.StringValue(result.Key)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result jiraProjectAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(state.Key.ValueString()))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(result.ID)
	state.Key = types.StringValue(result.Key)
	state.Name = types.StringValue(result.Name)
	state.Description = types.StringValue(result.Description)
	state.ProjectTypeKey = types.StringValue(result.ProjectTypeKey)
	state.LeadAccountID = types.StringValue(result.Lead.AccountID)
	state.AssigneeType = types.StringValue(result.AssigneeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := jiraProjectCreateRequest{
		Key:            plan.Key.ValueString(),
		Name:           plan.Name.ValueString(),
		Description:    plan.Description.ValueString(),
		ProjectTypeKey: plan.ProjectTypeKey.ValueString(),
		LeadAccountID:  plan.LeadAccountID.ValueString(),
		AssigneeType:   plan.AssigneeType.ValueString(),
	}

	var result jiraProjectAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(plan.Key.ValueString()))
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating project", err.Error())
		return
	}

	plan.ID = types.StringValue(result.ID)
	plan.Key = types.StringValue(result.Key)
	plan.Name = types.StringValue(result.Name)
	plan.Description = types.StringValue(result.Description)
	plan.ProjectTypeKey = types.StringValue(result.ProjectTypeKey)
	plan.LeadAccountID = types.StringValue(result.Lead.AccountID)
	plan.AssigneeType = types.StringValue(result.AssigneeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(state.Key.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting project", err.Error())
		return
	}

	// 404 means the project was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectKey := req.ID

	var result jiraProjectAPIResponse
	apiPath := fmt.Sprintf("/rest/api/3/project/%s", atlassian.PathEscape(projectKey))
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error importing project", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Project not found",
			fmt.Sprintf("No project found with key %q", projectKey),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), result.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), result.Key)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), result.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("description"), result.Description)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_type_key"), result.ProjectTypeKey)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("lead_account_id"), result.Lead.AccountID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("assignee_type"), result.AssigneeType)...)
}
