package jira

import (
	"context"
	"encoding/json"
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
	_ resource.Resource                = &screenResource{}
	_ resource.ResourceWithImportState = &screenResource{}
)

// NewScreenResource returns a new screen resource.
func NewScreenResource() resource.Resource {
	return &screenResource{}
}

type screenResource struct {
	client *atlassian.Client
}

type screenResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// screenAPIResponse represents the Jira screen API response shape.
type screenAPIResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// screenWriteRequest represents the Jira screen create/update request body.
type screenWriteRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *screenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen"
}

func (r *screenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud screen.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the screen.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the screen.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the screen.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
		},
	}
}

func (r *screenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *screenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan screenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := screenWriteRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	var result screenAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/screens", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating screen", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.ID))
	// Name and Description are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state screenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	screen, statusCode, err := r.findScreenByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading screen", err.Error())
		return
	}

	if statusCode == http.StatusNotFound || screen == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", screen.ID))
	state.Name = types.StringValue(screen.Name)
	state.Description = types.StringValue(screen.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *screenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan screenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state screenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := screenWriteRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s", atlassian.PathEscape(state.ID.ValueString()))
	var result screenAPIResponse
	err := r.client.Put(ctx, apiPath, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating screen", err.Error())
		return
	}

	// ID is carried forward from state (unchanged on update).
	// Name and Description are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *screenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state screenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/screens/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting screen", err.Error())
		return
	}

	// 404 means the screen was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *screenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findScreenByID paginates GET /rest/api/3/screens and returns the screen with the given ID.
// Returns (nil, http.StatusNotFound, nil) when the screen is not found after exhausting all pages.
func (r *screenResource) findScreenByID(ctx context.Context, id string) (*screenAPIResponse, int, error) {
	startAt := 0
	maxResults := 100

	for {
		apiPath := fmt.Sprintf("/rest/api/3/screens?maxResults=%d&startAt=%d", maxResults, startAt)

		var page struct {
			Values  []json.RawMessage `json:"values"`
			IsLast  bool              `json:"isLast"`
			StartAt int               `json:"startAt"`
			Total   int               `json:"total"`
		}

		statusCode, err := r.client.GetWithStatus(ctx, apiPath, &page)
		if err != nil {
			return nil, 0, err
		}
		if statusCode == http.StatusNotFound {
			return nil, http.StatusNotFound, nil
		}

		for _, raw := range page.Values {
			var s screenAPIResponse
			if err := json.Unmarshal(raw, &s); err != nil {
				return nil, 0, fmt.Errorf("unmarshaling screen: %w", err)
			}
			if fmt.Sprintf("%d", s.ID) == id {
				return &s, http.StatusOK, nil
			}
		}

		if page.IsLast || len(page.Values) == 0 {
			break
		}

		startAt += len(page.Values)
	}

	return nil, http.StatusNotFound, nil
}
