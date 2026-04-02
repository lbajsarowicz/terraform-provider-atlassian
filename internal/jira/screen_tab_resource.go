package jira

import (
	"context"
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
	_ resource.Resource                = &screenTabResource{}
	_ resource.ResourceWithImportState = &screenTabResource{}
)

// NewScreenTabResource returns a new screen tab resource.
func NewScreenTabResource() resource.Resource {
	return &screenTabResource{}
}

type screenTabResource struct {
	client *atlassian.Client
}

type screenTabResourceModel struct {
	ID       types.String `tfsdk:"id"`
	ScreenID types.String `tfsdk:"screen_id"`
	Name     types.String `tfsdk:"name"`
}

// screenTabAPIResponse represents the Jira screen tab API response shape.
type screenTabAPIResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// screenTabWriteRequest represents the Jira screen tab create/update request body.
type screenTabWriteRequest struct {
	Name string `json:"name"`
}

func (r *screenTabResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_tab"
}

func (r *screenTabResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a tab on a Jira Cloud screen.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the screen tab.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"screen_id": schema.StringAttribute{
				Description: "The ID of the screen this tab belongs to. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the screen tab.",
				Required:    true,
			},
		},
	}
}

func (r *screenTabResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *screenTabResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan screenTabResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := screenTabWriteRequest{
		Name: plan.Name.ValueString(),
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs", atlassian.PathEscape(plan.ScreenID.ValueString()))

	var result screenTabAPIResponse
	err := r.client.Post(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating screen tab", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	// ScreenID and Name are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenTabResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state screenTabResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs", atlassian.PathEscape(state.ScreenID.ValueString()))

	var tabs []screenTabAPIResponse
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &tabs)
	if err != nil {
		resp.Diagnostics.AddError("Error reading screen tabs", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	tabID := state.ID.ValueString()
	for _, tab := range tabs {
		if fmt.Sprintf("%d", tab.ID) == tabID {
			state.ID = types.StringValue(fmt.Sprintf("%d", tab.ID))
			state.Name = types.StringValue(tab.Name)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Tab not found — removed out-of-band.
	resp.State.RemoveResource(ctx)
}

func (r *screenTabResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan screenTabResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state screenTabResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := screenTabWriteRequest{
		Name: plan.Name.ValueString(),
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s",
		atlassian.PathEscape(state.ScreenID.ValueString()),
		atlassian.PathEscape(state.ID.ValueString()),
	)

	var result screenTabAPIResponse
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating screen tab", err.Error())
		return
	}

	// ID and ScreenID are carried forward from state (unchanged on update).
	// Name is preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenTabResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state screenTabResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s",
		atlassian.PathEscape(state.ScreenID.ValueString()),
		atlassian.PathEscape(state.ID.ValueString()),
	)

	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting screen tab", err.Error())
		return
	}

	// 404 means the screen tab was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *screenTabResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: {screenId}/{tabId}
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID in the format {screen_id}/{tab_id}, got %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("screen_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
