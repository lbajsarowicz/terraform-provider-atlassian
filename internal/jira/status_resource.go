package jira

import (
	"context"
	"encoding/json"
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
	_ resource.Resource                = &statusResource{}
	_ resource.ResourceWithImportState = &statusResource{}
)

// NewStatusResource returns a new status resource.
func NewStatusResource() resource.Resource {
	return &statusResource{}
}

type statusResource struct {
	client *atlassian.Client
}

type statusResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	StatusCategory types.String `tfsdk:"status_category"`
}

// statusAPIItem represents a single Jira status in API responses.
type statusAPIItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StatusCategory struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

type statusCreateRequest struct {
	Statuses []statusCreateItem `json:"statuses"`
	Scope    statusScope        `json:"scope"`
}

type statusScope struct {
	Type string `json:"type"`
}

type statusCreateItem struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	StatusCategory string `json:"statusCategory"`
}

type statusUpdateRequest struct {
	Statuses []statusUpdateItem `json:"statuses"`
}

type statusUpdateItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	StatusCategory string `json:"statusCategory"`
}

// statusSearchResponse represents the paginated search response for statuses.
type statusSearchResponse struct {
	StartAt    int             `json:"startAt"`
	MaxResults int             `json:"maxResults"`
	Total      int             `json:"total"`
	IsLast     bool            `json:"isLast"`
	Values     []statusAPIItem `json:"values"`
}

func (r *statusResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_status"
}

func (r *statusResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud status.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the status.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the status.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the status.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"status_category": schema.StringAttribute{
				Description: `The category of the status. Must be "TODO", "IN_PROGRESS", or "DONE". Changing this forces recreation of the resource.`,
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("TODO", "IN_PROGRESS", "DONE"),
				},
			},
		},
	}
}

func (r *statusResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *statusResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := statusCreateRequest{
		Statuses: []statusCreateItem{
			{
				Name:           plan.Name.ValueString(),
				Description:    plan.Description.ValueString(),
				StatusCategory: plan.StatusCategory.ValueString(),
			},
		},
		Scope: statusScope{Type: "GLOBAL"},
	}

	// POST response returns statusCategory as a flat string (e.g., "TODO"),
	// while GET returns it as a nested object ({"key": "TODO"}). Use a minimal
	// response type that only captures the ID we need.
	var result []struct {
		ID string `json:"id"`
	}
	err := r.client.Post(ctx, "/rest/api/3/statuses", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating status", err.Error())
		return
	}

	if len(result) == 0 {
		resp.Diagnostics.AddError("Error creating status", "API returned empty response")
		return
	}

	// Only take the server-assigned ID from the response; preserve plan values for all other fields.
	plan.ID = types.StringValue(result[0].ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	item, found, err := r.findStatusByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading status", err.Error())
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(item.ID)
	state.Name = types.StringValue(item.Name)
	state.Description = types.StringValue(item.Description)
	state.StatusCategory = types.StringValue(item.StatusCategory.Key)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state statusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := statusUpdateRequest{
		Statuses: []statusUpdateItem{
			{
				ID:             state.ID.ValueString(),
				Name:           plan.Name.ValueString(),
				Description:    plan.Description.ValueString(),
				StatusCategory: plan.StatusCategory.ValueString(),
			},
		},
	}

	err := r.client.Put(ctx, "/rest/api/3/statuses", body, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error updating status", err.Error())
		return
	}

	// Preserve plan values; ID is carried forward from state.
	plan.ID = state.ID

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete uses query param ?id=, NOT a path segment.
	apiPath := fmt.Sprintf("/rest/api/3/statuses?id=%s", atlassian.QueryEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting status", err.Error())
		return
	}

	// 404 means the status was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *statusResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findStatusByID paginates through GET /rest/api/3/statuses/search?id={id} and returns the matching item.
func (r *statusResource) findStatusByID(ctx context.Context, id string) (statusAPIItem, bool, error) {
	startAt := 0
	maxResults := 50

	for {
		apiPath := fmt.Sprintf(
			"/rest/api/3/statuses/search?id=%s&startAt=%d&maxResults=%d",
			atlassian.QueryEscape(id), startAt, maxResults,
		)

		var page statusSearchResponse
		statusCode, err := r.client.GetWithStatus(ctx, apiPath, &page)
		if err != nil {
			return statusAPIItem{}, false, err
		}
		if statusCode == http.StatusNotFound {
			return statusAPIItem{}, false, nil
		}

		for _, v := range page.Values {
			if v.ID == id {
				return v, true, nil
			}
		}

		if page.IsLast || len(page.Values) == 0 {
			break
		}

		startAt += len(page.Values)
	}

	return statusAPIItem{}, false, nil
}

// findAllStatuses paginates through all statuses and decodes them.
func findAllStatuses(ctx context.Context, client *atlassian.Client) ([]statusAPIItem, error) {
	rawValues, err := client.GetAllPages(ctx, "/rest/api/3/statuses/search")
	if err != nil {
		return nil, err
	}

	statuses := make([]statusAPIItem, 0, len(rawValues))
	for _, raw := range rawValues {
		var item statusAPIItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decoding status: %w", err)
		}
		statuses = append(statuses, item)
	}

	return statuses, nil
}
