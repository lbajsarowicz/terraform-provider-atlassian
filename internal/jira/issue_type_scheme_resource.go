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
	_ resource.Resource                = &issueTypeSchemeResource{}
	_ resource.ResourceWithImportState = &issueTypeSchemeResource{}
)

// NewIssueTypeSchemeResource returns a new issue type scheme resource.
func NewIssueTypeSchemeResource() resource.Resource {
	return &issueTypeSchemeResource{}
}

type issueTypeSchemeResource struct {
	client *atlassian.Client
}

type issueTypeSchemeResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	DefaultIssueTypeID types.String `tfsdk:"default_issue_type_id"`
	IssueTypeIDs       types.List   `tfsdk:"issue_type_ids"`
}

// issueTypeSchemeCreateRequest represents the POST request body.
type issueTypeSchemeCreateRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	DefaultIssueTypeID string   `json:"defaultIssueTypeId,omitempty"`
	IssueTypeIDs       []string `json:"issueTypeIds"`
}

// issueTypeSchemeCreateResponse represents the POST response body.
type issueTypeSchemeCreateResponse struct {
	IssueTypeSchemeID string `json:"issueTypeSchemeId"`
}

// issueTypeSchemeUpdateRequest represents the PUT request body for updating name/description/default.
type issueTypeSchemeUpdateRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	DefaultIssueTypeID string `json:"defaultIssueTypeId,omitempty"`
}

// issueTypeSchemeAPIResponse represents a single scheme in the paginated list response.
type issueTypeSchemeAPIResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	DefaultIssueTypeID string `json:"defaultIssueTypeId,omitempty"`
	IsDefault          bool   `json:"isDefault"`
}

// issueTypeSchemeItemResponse represents a single item mapping from the items endpoint.
type issueTypeSchemeItemResponse struct {
	IssueTypeSchemeID string `json:"issueTypeSchemeId"`
	IssueTypeID       string `json:"issueTypeId"`
}

func (r *issueTypeSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_scheme"
}

func (r *issueTypeSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud issue type scheme.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the issue type scheme.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue type scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the issue type scheme.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"default_issue_type_id": schema.StringAttribute{
				Description: "The ID of the default issue type of the issue type scheme.",
				Optional:    true,
			},
			"issue_type_ids": schema.ListAttribute{
				Description: "An ordered list of issue type IDs in the scheme. Order is preserved.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *issueTypeSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *issueTypeSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan issueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var issueTypeIDs []string
	resp.Diagnostics.Append(plan.IssueTypeIDs.ElementsAs(ctx, &issueTypeIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := issueTypeSchemeCreateRequest{
		Name:         plan.Name.ValueString(),
		Description:  plan.Description.ValueString(),
		IssueTypeIDs: issueTypeIDs,
	}
	if !plan.DefaultIssueTypeID.IsNull() && !plan.DefaultIssueTypeID.IsUnknown() {
		body.DefaultIssueTypeID = plan.DefaultIssueTypeID.ValueString()
	}

	var result issueTypeSchemeCreateResponse
	err := r.client.Post(ctx, "/rest/api/3/issuetypescheme", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating issue type scheme", err.Error())
		return
	}

	plan.ID = types.StringValue(result.IssueTypeSchemeID)

	// Preserve plan values for name, description, default_issue_type_id, issue_type_ids
	// (POST returns partial response)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *issueTypeSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state issueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schemeID := state.ID.ValueString()

	// Fetch scheme details by listing all and finding ours
	scheme, found, err := r.findSchemeByID(ctx, schemeID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type scheme", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(scheme.ID)
	state.Name = types.StringValue(scheme.Name)
	state.Description = types.StringValue(scheme.Description)
	if scheme.DefaultIssueTypeID != "" {
		state.DefaultIssueTypeID = types.StringValue(scheme.DefaultIssueTypeID)
	} else {
		state.DefaultIssueTypeID = types.StringNull()
	}

	// Fetch issue type IDs via the items/mapping endpoint
	issueTypeIDs, err := r.getSchemeIssueTypeIDs(ctx, schemeID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type scheme items", err.Error())
		return
	}

	listValue, diags := types.ListValueFrom(ctx, types.StringType, issueTypeIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.IssueTypeIDs = listValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *issueTypeSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan issueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state issueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schemeID := state.ID.ValueString()

	// Update scheme properties (name, description, defaultIssueTypeId)
	updateBody := issueTypeSchemeUpdateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}
	if !plan.DefaultIssueTypeID.IsNull() && !plan.DefaultIssueTypeID.IsUnknown() {
		updateBody.DefaultIssueTypeID = plan.DefaultIssueTypeID.ValueString()
	}

	apiPath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s", atlassian.PathEscape(schemeID))
	err := r.client.Put(ctx, apiPath, updateBody, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error updating issue type scheme", err.Error())
		return
	}

	// Update issue type IDs if they changed
	var planIssueTypeIDs []string
	resp.Diagnostics.Append(plan.IssueTypeIDs.ElementsAs(ctx, &planIssueTypeIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateIssueTypeIDs []string
	resp.Diagnostics.Append(state.IssueTypeIDs.ElementsAs(ctx, &stateIssueTypeIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !issueTypeIDsEqual(stateIssueTypeIDs, planIssueTypeIDs) {
		// Compute which to remove and which to add
		stateSet := make(map[string]bool, len(stateIssueTypeIDs))
		for _, id := range stateIssueTypeIDs {
			stateSet[id] = true
		}
		planSet := make(map[string]bool, len(planIssueTypeIDs))
		for _, id := range planIssueTypeIDs {
			planSet[id] = true
		}

		// Remove issue types not in the plan
		for _, id := range stateIssueTypeIDs {
			if !planSet[id] {
				delPath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s/issuetype/%s",
					atlassian.PathEscape(schemeID),
					atlassian.PathEscape(id),
				)
				_, delErr := r.client.DeleteWithStatus(ctx, delPath)
				if delErr != nil {
					resp.Diagnostics.AddError("Error removing issue type from scheme", delErr.Error())
					return
				}
			}
		}

		// Add issue types not in the current state
		var toAdd []string
		for _, id := range planIssueTypeIDs {
			if !stateSet[id] {
				toAdd = append(toAdd, id)
			}
		}
		if len(toAdd) > 0 {
			addPath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s/issuetype", atlassian.PathEscape(schemeID))
			addBody := map[string][]string{
				"issueTypeIds": toAdd,
			}
			addErr := r.client.Put(ctx, addPath, addBody, nil)
			if addErr != nil {
				resp.Diagnostics.AddError("Error adding issue types to scheme", addErr.Error())
				return
			}
		}

		// Reorder to match plan order
		movePath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s/issuetype/move", atlassian.PathEscape(schemeID))
		moveBody := map[string]interface{}{
			"issueTypeIds": planIssueTypeIDs,
			"position":     "First",
		}
		moveErr := r.client.Put(ctx, movePath, moveBody, nil)
		if moveErr != nil {
			resp.Diagnostics.AddError("Error reordering issue types in scheme", moveErr.Error())
			return
		}
	}

	plan.ID = state.ID

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *issueTypeSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state issueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/issuetypescheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting issue type scheme", err.Error())
		return
	}

	// 404 means the scheme was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *issueTypeSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findSchemeByID fetches all issue type schemes and returns the one with the given ID.
func (r *issueTypeSchemeResource) findSchemeByID(ctx context.Context, schemeID string) (*issueTypeSchemeAPIResponse, bool, error) {
	allValues, err := r.client.GetAllPages(ctx, "/rest/api/3/issuetypescheme")
	if err != nil {
		return nil, false, err
	}

	for _, raw := range allValues {
		var scheme issueTypeSchemeAPIResponse
		if err := json.Unmarshal(raw, &scheme); err != nil {
			return nil, false, fmt.Errorf("unmarshaling issue type scheme: %w", err)
		}
		if scheme.ID == schemeID {
			return &scheme, true, nil
		}
	}

	return nil, false, nil
}

// getSchemeIssueTypeIDs fetches the ordered list of issue type IDs for a scheme.
func (r *issueTypeSchemeResource) getSchemeIssueTypeIDs(ctx context.Context, schemeID string) ([]string, error) {
	itemsPath := fmt.Sprintf("/rest/api/3/issuetypescheme/mapping?issueTypeSchemeId=%s", atlassian.QueryEscape(schemeID))
	allValues, err := r.client.GetAllPages(ctx, itemsPath)
	if err != nil {
		return nil, fmt.Errorf("fetching issue type scheme items: %w", err)
	}

	var issueTypeIDs []string
	for _, raw := range allValues {
		var item issueTypeSchemeItemResponse
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshaling issue type scheme item: %w", err)
		}
		if item.IssueTypeSchemeID == schemeID {
			issueTypeIDs = append(issueTypeIDs, item.IssueTypeID)
		}
	}

	return issueTypeIDs, nil
}

// issueTypeIDsEqual checks if two string slices are equal (same length and same elements in same order).
func issueTypeIDsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
