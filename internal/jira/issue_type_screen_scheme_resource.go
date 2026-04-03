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
	_ resource.Resource                = &issueTypeScreenSchemeResource{}
	_ resource.ResourceWithImportState = &issueTypeScreenSchemeResource{}
)

// NewIssueTypeScreenSchemeResource returns a new issue type screen scheme resource.
func NewIssueTypeScreenSchemeResource() resource.Resource {
	return &issueTypeScreenSchemeResource{}
}

type issueTypeScreenSchemeResource struct {
	client *atlassian.Client
}

type issueTypeScreenSchemeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Mappings    types.Map    `tfsdk:"mappings"`
}

// issueTypeScreenSchemeAPIResponse represents a single scheme in the paginated list response.
type issueTypeScreenSchemeAPIResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// issueTypeScreenSchemeCreateRequest represents the POST request body.
type issueTypeScreenSchemeCreateRequest struct {
	Name              string                             `json:"name"`
	Description       string                             `json:"description,omitempty"`
	IssueTypeMappings []issueTypeScreenSchemeMappingItem `json:"issueTypeMappings"`
}

// issueTypeScreenSchemeCreateResponse represents the POST response body.
// The API returns {"id": "..."}, not {"issueTypeScreenSchemeId": "..."}.
type issueTypeScreenSchemeCreateResponse struct {
	ID string `json:"id"`
}

// issueTypeScreenSchemeUpdateRequest represents the PUT request body for name/description updates.
type issueTypeScreenSchemeUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// issueTypeScreenSchemeMappingItem is used in create/update mapping requests.
type issueTypeScreenSchemeMappingItem struct {
	IssueTypeID    string `json:"issueTypeId"`
	ScreenSchemeID string `json:"screenSchemeId"`
}

// issueTypeScreenSchemeMappingResponse is the shape of items returned by the mapping endpoint.
type issueTypeScreenSchemeMappingResponse struct {
	IssueTypeScreenSchemeID string `json:"issueTypeScreenSchemeId"`
	IssueTypeID             string `json:"issueTypeId"`
	ScreenSchemeID          string `json:"screenSchemeId"`
}

func (r *issueTypeScreenSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_screen_scheme"
}

func (r *issueTypeScreenSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud issue type screen scheme.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the issue type screen scheme.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue type screen scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the issue type screen scheme.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"mappings": schema.MapAttribute{
				Description: `A map of issue type IDs to screen scheme IDs. Use "default" as the key for the default screen scheme.`,
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *issueTypeScreenSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *issueTypeScreenSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan issueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mappings := make(map[string]string)
	resp.Diagnostics.Append(plan.Mappings.ElementsAs(ctx, &mappings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var mappingItems []issueTypeScreenSchemeMappingItem
	for issueTypeID, screenSchemeID := range mappings {
		mappingItems = append(mappingItems, issueTypeScreenSchemeMappingItem{
			IssueTypeID:    issueTypeID,
			ScreenSchemeID: screenSchemeID,
		})
	}

	body := issueTypeScreenSchemeCreateRequest{
		Name:              plan.Name.ValueString(),
		Description:       plan.Description.ValueString(),
		IssueTypeMappings: mappingItems,
	}

	var result issueTypeScreenSchemeCreateResponse
	err := r.client.Post(ctx, "/rest/api/3/issuetypescreenscheme", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating issue type screen scheme", err.Error())
		return
	}

	plan.ID = types.StringValue(result.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *issueTypeScreenSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state issueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schemeID := state.ID.ValueString()

	scheme, found, err := r.findSchemeByID(ctx, schemeID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type screen scheme", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(scheme.ID)
	state.Name = types.StringValue(scheme.Name)
	state.Description = types.StringValue(scheme.Description)

	// Fetch current mappings
	mappingsMap, err := r.getMappings(ctx, schemeID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type screen scheme mappings", err.Error())
		return
	}

	mappingsValue, diags := types.MapValueFrom(ctx, types.StringType, mappingsMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Mappings = mappingsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *issueTypeScreenSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan issueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state issueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schemeID := state.ID.ValueString()

	// Update name and description
	updateBody := issueTypeScreenSchemeUpdateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}
	apiPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", atlassian.PathEscape(schemeID))
	err := r.client.Put(ctx, apiPath, updateBody, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error updating issue type screen scheme", err.Error())
		return
	}

	// Compute mapping diff
	planMappings := make(map[string]string)
	resp.Diagnostics.Append(plan.Mappings.ElementsAs(ctx, &planMappings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateMappings := make(map[string]string)
	resp.Diagnostics.Append(state.Mappings.ElementsAs(ctx, &stateMappings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine mappings to add/update (in plan but not in state, or different screenSchemeId)
	var toAddOrUpdate []issueTypeScreenSchemeMappingItem
	for issueTypeID, screenSchemeID := range planMappings {
		if existing, ok := stateMappings[issueTypeID]; !ok || existing != screenSchemeID {
			toAddOrUpdate = append(toAddOrUpdate, issueTypeScreenSchemeMappingItem{
				IssueTypeID:    issueTypeID,
				ScreenSchemeID: screenSchemeID,
			})
		}
	}

	if len(toAddOrUpdate) > 0 {
		addPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s/mapping", atlassian.PathEscape(schemeID))
		addBody := map[string][]issueTypeScreenSchemeMappingItem{
			"issueTypeMappings": toAddOrUpdate,
		}
		if err := r.client.Put(ctx, addPath, addBody, nil); err != nil {
			resp.Diagnostics.AddError("Error adding issue type screen scheme mappings", err.Error())
			return
		}
	}

	// Determine mappings to remove (in state but not in plan); skip "default"
	var toRemove []string
	for issueTypeID := range stateMappings {
		if issueTypeID == "default" {
			continue
		}
		if _, ok := planMappings[issueTypeID]; !ok {
			toRemove = append(toRemove, issueTypeID)
		}
	}

	if len(toRemove) > 0 {
		removePath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s/mapping/remove", atlassian.PathEscape(schemeID))
		removeBody := map[string][]string{
			"issueTypeIds": toRemove,
		}
		if err := r.client.Post(ctx, removePath, removeBody, nil); err != nil {
			resp.Diagnostics.AddError("Error removing issue type screen scheme mappings", err.Error())
			return
		}
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *issueTypeScreenSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state issueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting issue type screen scheme", err.Error())
		return
	}

	// 404 means the scheme was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *issueTypeScreenSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findSchemeByID fetches all issue type screen schemes and returns the one with the given ID.
func (r *issueTypeScreenSchemeResource) findSchemeByID(ctx context.Context, schemeID string) (*issueTypeScreenSchemeAPIResponse, bool, error) {
	allValues, err := r.client.GetAllPages(ctx, "/rest/api/3/issuetypescreenscheme")
	if err != nil {
		return nil, false, err
	}

	for _, raw := range allValues {
		var scheme issueTypeScreenSchemeAPIResponse
		if err := json.Unmarshal(raw, &scheme); err != nil {
			return nil, false, fmt.Errorf("unmarshaling issue type screen scheme: %w", err)
		}
		if scheme.ID == schemeID {
			return &scheme, true, nil
		}
	}

	return nil, false, nil
}

// getMappings fetches current mappings for the given scheme ID and returns issueTypeId -> screenSchemeId.
func (r *issueTypeScreenSchemeResource) getMappings(ctx context.Context, schemeID string) (map[string]string, error) {
	mappingPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/mapping?issueTypeScreenSchemeId=%s", atlassian.QueryEscape(schemeID))
	allValues, err := r.client.GetAllPages(ctx, mappingPath)
	if err != nil {
		return nil, fmt.Errorf("fetching issue type screen scheme mappings: %w", err)
	}

	result := make(map[string]string)
	for _, raw := range allValues {
		var item issueTypeScreenSchemeMappingResponse
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshaling issue type screen scheme mapping: %w", err)
		}
		if item.IssueTypeScreenSchemeID == schemeID {
			result[item.IssueTypeID] = item.ScreenSchemeID
		}
	}

	return result, nil
}
