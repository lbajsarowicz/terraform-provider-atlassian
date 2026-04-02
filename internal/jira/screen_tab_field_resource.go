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
	_ resource.Resource                = &screenTabFieldResource{}
	_ resource.ResourceWithImportState = &screenTabFieldResource{}
)

// NewScreenTabFieldResource returns a new screen tab field resource.
func NewScreenTabFieldResource() resource.Resource {
	return &screenTabFieldResource{}
}

type screenTabFieldResource struct {
	client *atlassian.Client
}

type screenTabFieldResourceModel struct {
	ScreenID types.String `tfsdk:"screen_id"`
	TabID    types.String `tfsdk:"tab_id"`
	FieldID  types.String `tfsdk:"field_id"`
}

// screenTabFieldAPIResponse represents the Jira screen tab field API response shape.
type screenTabFieldAPIResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// screenTabFieldWriteRequest represents the request body for adding a field to a tab.
type screenTabFieldWriteRequest struct {
	FieldID string `json:"fieldId"`
}

func (r *screenTabFieldResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_tab_field"
}

func (r *screenTabFieldResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Adds a field to a tab on a Jira Cloud screen. All attributes are immutable; any change forces recreation.",
		Attributes: map[string]schema.Attribute{
			"screen_id": schema.StringAttribute{
				Description: "The ID of the screen. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tab_id": schema.StringAttribute{
				Description: "The ID of the screen tab. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"field_id": schema.StringAttribute{
				Description: "The ID of the field to add to the tab. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *screenTabFieldResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *screenTabFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan screenTabFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := screenTabFieldWriteRequest{
		FieldID: plan.FieldID.ValueString(),
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/fields",
		atlassian.PathEscape(plan.ScreenID.ValueString()),
		atlassian.PathEscape(plan.TabID.ValueString()),
	)

	var result screenTabFieldAPIResponse
	err := r.client.Post(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error adding field to screen tab", err.Error())
		return
	}

	// All attributes are preserved from the plan (user intent).
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenTabFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state screenTabFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/fields",
		atlassian.PathEscape(state.ScreenID.ValueString()),
		atlassian.PathEscape(state.TabID.ValueString()),
	)

	var fields []screenTabFieldAPIResponse
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &fields)
	if err != nil {
		resp.Diagnostics.AddError("Error reading screen tab fields", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	fieldID := state.FieldID.ValueString()
	for _, f := range fields {
		if f.ID == fieldID {
			// Field found — state is unchanged.
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Field not found — removed out-of-band.
	resp.State.RemoveResource(ctx)
}

func (r *screenTabFieldResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes are ForceNew; Update is never called by the framework.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Changing screen_id, tab_id, or field_id requires replacing the resource.",
	)
}

func (r *screenTabFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state screenTabFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/fields/%s",
		atlassian.PathEscape(state.ScreenID.ValueString()),
		atlassian.PathEscape(state.TabID.ValueString()),
		atlassian.PathEscape(state.FieldID.ValueString()),
	)

	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error removing field from screen tab", err.Error())
		return
	}

	// 404 means the field was already removed out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *screenTabFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: {screenId}/{tabId}/{fieldId}
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID in the format {screen_id}/{tab_id}/{field_id}, got %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("screen_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tab_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("field_id"), parts[2])...)
}
