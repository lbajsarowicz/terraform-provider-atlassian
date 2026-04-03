package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &spacePermissionResource{}
	_ resource.ResourceWithImportState = &spacePermissionResource{}
)

// NewSpacePermissionResource returns a new Confluence space permission resource.
func NewSpacePermissionResource() resource.Resource {
	return &spacePermissionResource{}
}

type spacePermissionResource struct {
	client *atlassian.Client
}

type spacePermissionResourceModel struct {
	ID              types.String `tfsdk:"id"`
	SpaceKey        types.String `tfsdk:"space_key"`
	SpaceID         types.String `tfsdk:"space_id"`
	PrincipalType   types.String `tfsdk:"principal_type"`
	PrincipalID     types.String `tfsdk:"principal_id"`
	OperationKey    types.String `tfsdk:"operation_key"`
	OperationTarget types.String `tfsdk:"operation_target"`
}

// spacePermissionCreateRequest is the body for POST /wiki/rest/api/space/{key}/permission (v1).
type spacePermissionCreateRequest struct {
	Subject struct {
		Type       string `json:"type"`
		Identifier string `json:"identifier"`
	} `json:"subject"`
	Operation struct {
		Key    string `json:"key"`
		Target string `json:"target"`
	} `json:"operation"`
}

// spacePermissionV1Response is the response from POST /wiki/rest/api/space/{key}/permission.
type spacePermissionV1Response struct {
	ID json.Number `json:"id"`
}

// spacePermissionV2Item is a single item from GET /wiki/api/v2/spaces/{id}/permissions.
type spacePermissionV2Item struct {
	ID        string `json:"id"`
	Principal struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"principal"`
	Operation struct {
		Key        string `json:"key"`
		TargetType string `json:"targetType"`
	} `json:"operation"`
}

func (r *spacePermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space_permission"
}

func (r *spacePermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Confluence Cloud space permission. All attributes except id are ForceNew.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The numeric ID of the permission.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_key": schema.StringAttribute{
				Description: "The key of the space this permission belongs to. Changing this forces recreation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The numeric ID of the space. Required to read permissions via the v2 API. Changing this forces recreation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_type": schema.StringAttribute{
				Description: `The principal type: "user" or "group". Changing this forces recreation.`,
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_id": schema.StringAttribute{
				Description: "The account ID (for users) or group ID (for groups). Changing this forces recreation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"operation_key": schema.StringAttribute{
				Description: `The permission operation key (e.g. "read", "create", "delete"). Changing this forces recreation.`,
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"operation_target": schema.StringAttribute{
				Description: `The operation target (e.g. "space", "page", "comment"). Changing this forces recreation.`,
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *spacePermissionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *spacePermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan spacePermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var body spacePermissionCreateRequest
	body.Subject.Type = plan.PrincipalType.ValueString()
	body.Subject.Identifier = plan.PrincipalID.ValueString()
	body.Operation.Key = plan.OperationKey.ValueString()
	body.Operation.Target = plan.OperationTarget.ValueString()

	apiPath := fmt.Sprintf("/wiki/rest/api/space/%s/permission", atlassian.PathEscape(plan.SpaceKey.ValueString()))

	var result spacePermissionV1Response
	if err := r.client.Post(ctx, apiPath, body, &result); err != nil {
		resp.Diagnostics.AddError("Error creating Confluence space permission", err.Error())
		return
	}

	plan.ID = types.StringValue(result.ID.String())

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spacePermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state spacePermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permID := state.ID.ValueString()
	spaceID := state.SpaceID.ValueString()

	apiPath := fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", atlassian.PathEscape(spaceID))
	allPerms, statusCode, err := r.client.GetAllPagesCursorWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Confluence space permissions", err.Error())
		return
	}
	if statusCode == http.StatusNotFound {
		// Space was deleted out-of-band; remove the permission from state too.
		resp.State.RemoveResource(ctx)
		return
	}

	for _, raw := range allPerms {
		var perm spacePermissionV2Item
		if err := json.Unmarshal(raw, &perm); err != nil {
			continue
		}
		if perm.ID == permID {
			// Permission still exists; update state from API response.
			state.PrincipalType = types.StringValue(perm.Principal.Type)
			state.PrincipalID = types.StringValue(perm.Principal.ID)
			state.OperationKey = types.StringValue(perm.Operation.Key)
			state.OperationTarget = types.StringValue(perm.Operation.TargetType)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Permission not found — removed out-of-band.
	resp.State.RemoveResource(ctx)
}

func (r *spacePermissionResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"All Confluence space permission attributes are ForceNew; update should never be called.",
	)
}

func (r *spacePermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state spacePermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/wiki/rest/api/space/%s/permission/%s",
		atlassian.PathEscape(state.SpaceKey.ValueString()),
		atlassian.PathEscape(state.ID.ValueString()),
	)

	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting Confluence space permission", err.Error())
		return
	}

	// 404 means already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *spacePermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by "spaceKey/spaceId/permissionId", e.g. "MYSPACE/196611/42".
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format: spaceKey/spaceId/permissionId, got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_key"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}
