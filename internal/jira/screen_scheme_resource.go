package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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
	_ resource.Resource                = &screenSchemeResource{}
	_ resource.ResourceWithImportState = &screenSchemeResource{}
)

// NewScreenSchemeResource returns a new screen scheme resource.
func NewScreenSchemeResource() resource.Resource {
	return &screenSchemeResource{}
}

type screenSchemeResource struct {
	client *atlassian.Client
}

type screenSchemeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Screens     types.Map    `tfsdk:"screens"` // operation -> screen_id (string)
}

// screenSchemeAPIResponse represents the Jira screen scheme API response shape.
type screenSchemeAPIResponse struct {
	ID          int64            `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Screens     map[string]int64 `json:"screens"` // operation -> screen ID (int64 in JSON)
}

// screenSchemeCreateRequest represents the Jira screen scheme create/update request body.
type screenSchemeCreateRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Screens     map[string]int64 `json:"screens"`
}

func (r *screenSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_scheme"
}

func (r *screenSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud screen scheme.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the screen scheme.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the screen scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the screen scheme.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"screens": schema.MapAttribute{
				Description: `A map of issue operation names to screen IDs. Valid keys: "default", "create", "view", "edit". The "default" key is required.`,
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *screenSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *screenSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan screenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	screensInt, err := screensMapToInt64(ctx, plan.Screens)
	if err != nil {
		resp.Diagnostics.AddError("Error parsing screens map", err.Error())
		return
	}

	body := screenSchemeCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Screens:     screensInt,
	}

	var result screenSchemeAPIResponse
	if err := r.client.Post(ctx, "/rest/api/3/screenscheme", body, &result); err != nil {
		resp.Diagnostics.AddError("Error creating screen scheme", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state screenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scheme, found, err := r.findScreenSchemeByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading screen scheme", err.Error())
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", scheme.ID))
	state.Name = types.StringValue(scheme.Name)
	state.Description = types.StringValue(scheme.Description)

	screensStr := screensInt64ToStr(scheme.Screens)
	screensValue, diags := types.MapValueFrom(ctx, types.StringType, screensStr)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Screens = screensValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *screenSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan screenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state screenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	screensInt, err := screensMapToInt64(ctx, plan.Screens)
	if err != nil {
		resp.Diagnostics.AddError("Error parsing screens map", err.Error())
		return
	}

	body := screenSchemeCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Screens:     screensInt,
	}

	apiPath := fmt.Sprintf("/rest/api/3/screenscheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	if err := r.client.Put(ctx, apiPath, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating screen scheme", err.Error())
		return
	}

	// ID carries forward from state (unchanged on update).
	plan.ID = state.ID

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state screenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/screenscheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting screen scheme", err.Error())
		return
	}

	// 404 means the screen scheme was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *screenSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findScreenSchemeByID paginates GET /rest/api/3/screenscheme and returns the scheme with the given ID.
func (r *screenSchemeResource) findScreenSchemeByID(ctx context.Context, id string) (*screenSchemeAPIResponse, bool, error) {
	allValues, err := r.client.GetAllPages(ctx, "/rest/api/3/screenscheme")
	if err != nil {
		return nil, false, err
	}

	for _, raw := range allValues {
		var s screenSchemeAPIResponse
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, fmt.Errorf("unmarshaling screen scheme: %w", err)
		}
		if fmt.Sprintf("%d", s.ID) == id {
			return &s, true, nil
		}
	}

	return nil, false, nil
}

// screensInt64ToStr converts a map[string]int64 screens map to map[string]string for Terraform state.
func screensInt64ToStr(screens map[string]int64) map[string]string {
	result := make(map[string]string, len(screens))
	for k, v := range screens {
		result[k] = fmt.Sprintf("%d", v)
	}
	return result
}

// screensMapToInt64 converts a types.Map of screens (string values) back to map[string]int64 for API requests.
func screensMapToInt64(ctx context.Context, screens types.Map) (map[string]int64, error) {
	strMap := make(map[string]string)
	if diags := screens.ElementsAs(ctx, &strMap, false); diags.HasError() {
		return nil, fmt.Errorf("converting screens map elements")
	}

	result := make(map[string]int64, len(strMap))
	for k, v := range strMap {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid screen ID %q for operation %q: %w", v, k, err)
		}
		result[k] = id
	}
	return result, nil
}
