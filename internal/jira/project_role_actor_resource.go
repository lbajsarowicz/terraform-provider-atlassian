package jira

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &projectRoleActorResource{}
	_ resource.ResourceWithImportState = &projectRoleActorResource{}
)

// NewProjectRoleActorResource returns a new project role actor resource.
func NewProjectRoleActorResource() resource.Resource {
	return &projectRoleActorResource{}
}

type projectRoleActorResource struct {
	client *atlassian.Client
}

type projectRoleActorResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectKey types.String `tfsdk:"project_key"`
	RoleID     types.String `tfsdk:"role_id"`
	ActorType  types.String `tfsdk:"actor_type"`
	ActorValue types.String `tfsdk:"actor_value"`
}

// projectRoleActorsAPIResponse represents the GET response for project role actors.
type projectRoleActorsAPIResponse struct {
	ID     int                         `json:"id"`
	Name   string                      `json:"name"`
	Actors []projectRoleActorAPIObject `json:"actors"`
}

// projectRoleActorAPIObject represents a single actor in the API response.
type projectRoleActorAPIObject struct {
	ID          int    `json:"id"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	ActorUser   *struct {
		AccountID string `json:"accountId"`
	} `json:"actorUser,omitempty"`
	ActorGroup *struct {
		DisplayName string `json:"displayName"`
		Name        string `json:"name"`
		GroupID     string `json:"groupId"`
	} `json:"actorGroup,omitempty"`
}

func (r *projectRoleActorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_role_actor"
}

func (r *projectRoleActorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Assigns a user or group to a project role in a specific Jira Cloud project. This resource is immutable; any change forces recreation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Composite ID: {project_key}/{role_id}/{actor_type}/{actor_value}.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_key": schema.StringAttribute{
				Description: "The key of the project.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role_id": schema.StringAttribute{
				Description: "The ID of the project role.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"actor_type": schema.StringAttribute{
				Description: "The type of actor: atlassianUser or atlassianGroup.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("atlassianUser", "atlassianGroup"),
				},
			},
			"actor_value": schema.StringAttribute{
				Description: "The actor value: accountId for users, group name for groups.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *projectRoleActorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectRoleActorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectRoleActorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/project/%s/role/%s",
		atlassian.PathEscape(plan.ProjectKey.ValueString()),
		atlassian.PathEscape(plan.RoleID.ValueString()),
	)

	// Build the request body: POST uses "user" or "group" keys with arrays
	var body map[string][]string
	switch plan.ActorType.ValueString() {
	case "atlassianUser":
		body = map[string][]string{
			"user": {plan.ActorValue.ValueString()},
		}
	case "atlassianGroup":
		body = map[string][]string{
			"group": {plan.ActorValue.ValueString()},
		}
	}

	// POST returns the full role with all actors; we don't need to decode it for state.
	err := r.client.Post(ctx, apiPath, body, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error adding project role actor", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s/%s/%s",
		plan.ProjectKey.ValueString(),
		plan.RoleID.ValueString(),
		plan.ActorType.ValueString(),
		plan.ActorValue.ValueString(),
	))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectRoleActorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectRoleActorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/project/%s/role/%s",
		atlassian.PathEscape(state.ProjectKey.ValueString()),
		atlassian.PathEscape(state.RoleID.ValueString()),
	)

	var result projectRoleActorsAPIResponse
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project role actors", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	// Find our specific actor in the list
	found := r.findActor(result.Actors, state.ActorType.ValueString(), state.ActorValue.ValueString())
	if !found {
		// Actor was removed out-of-band
		resp.State.RemoveResource(ctx)
		return
	}

	// State is unchanged — preserve plan values
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectRoleActorResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Project role actors are immutable. All attribute changes require replacing the resource.",
	)
}

func (r *projectRoleActorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectRoleActorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// DELETE uses query parameters for actor identification
	var queryParam string
	switch state.ActorType.ValueString() {
	case "atlassianUser":
		queryParam = fmt.Sprintf("user=%s", atlassian.QueryEscape(state.ActorValue.ValueString()))
	case "atlassianGroup":
		queryParam = fmt.Sprintf("group=%s", atlassian.QueryEscape(state.ActorValue.ValueString()))
	}

	apiPath := fmt.Sprintf("/rest/api/3/project/%s/role/%s?%s",
		atlassian.PathEscape(state.ProjectKey.ValueString()),
		atlassian.PathEscape(state.RoleID.ValueString()),
		queryParam,
	)

	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error removing project role actor", err.Error())
		return
	}

	// 404 means the actor was already removed out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *projectRoleActorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: {projectKey}/{roleId}/{actorType}/{actorValue}
	parts := strings.SplitN(req.ID, "/", 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID in the format {project_key}/{role_id}/{actor_type}/{actor_value}, got %q", req.ID),
		)
		return
	}

	projectKey := parts[0]
	roleID := parts[1]
	actorType := parts[2]
	actorValue := parts[3]

	if actorType != "atlassianUser" && actorType != "atlassianGroup" {
		resp.Diagnostics.AddError(
			"Invalid actor type",
			fmt.Sprintf("actor_type must be atlassianUser or atlassianGroup, got %q", actorType),
		)
		return
	}

	// Verify the actor exists
	apiPath := fmt.Sprintf("/rest/api/3/project/%s/role/%s",
		atlassian.PathEscape(projectKey),
		atlassian.PathEscape(roleID),
	)

	var result projectRoleActorsAPIResponse
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error importing project role actor", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Project role not found",
			fmt.Sprintf("No project role found for project %q with role ID %q", projectKey, roleID),
		)
		return
	}

	found := r.findActor(result.Actors, actorType, actorValue)
	if !found {
		resp.Diagnostics.AddError(
			"Actor not found",
			fmt.Sprintf("No actor of type %q with value %q found in project %q role %q", actorType, actorValue, projectKey, roleID),
		)
		return
	}

	compositeID := fmt.Sprintf("%s/%s/%s/%s", projectKey, roleID, actorType, actorValue)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), compositeID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_key"), projectKey)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), roleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("actor_type"), actorType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("actor_value"), actorValue)...)
}

// findActor checks whether the given actor exists in the actors list.
func (r *projectRoleActorResource) findActor(actors []projectRoleActorAPIObject, actorType, actorValue string) bool {
	for _, actor := range actors {
		if actor.Type == "atlassian-user-role-actor" && actorType == "atlassianUser" {
			if actor.ActorUser != nil && actor.ActorUser.AccountID == actorValue {
				return true
			}
		}
		if actor.Type == "atlassian-group-role-actor" && actorType == "atlassianGroup" {
			if actor.ActorGroup != nil && actor.ActorGroup.Name == actorValue {
				return true
			}
		}
	}
	return false
}
