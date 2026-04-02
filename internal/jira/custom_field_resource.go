package jira

import (
	"context"
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
	_ resource.Resource                = &customFieldResource{}
	_ resource.ResourceWithImportState = &customFieldResource{}
)

// NewCustomFieldResource returns a new custom field resource.
func NewCustomFieldResource() resource.Resource {
	return &customFieldResource{}
}

type customFieldResource struct {
	client *atlassian.Client
}

type customFieldResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	SearcherKey types.String `tfsdk:"searcher_key"`
}

// customFieldAPIResponse represents a single field from the GET /rest/api/3/field list.
type customFieldAPIResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Custom      bool   `json:"custom"`
	Description string `json:"description"`
	SearcherKey string `json:"searcherKey"`
	Schema      struct {
		Type     string `json:"type"`
		Custom   string `json:"custom"`
		CustomID int64  `json:"customId"`
	} `json:"schema"`
}

// customFieldCreateRequest represents the POST request body for creating a custom field.
type customFieldCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	SearcherKey string `json:"searcherKey,omitempty"`
}

// customFieldUpdateRequest represents the PUT request body for updating a custom field.
type customFieldUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *customFieldResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_custom_field"
}

func (r *customFieldResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira Cloud custom field.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the custom field (e.g. customfield_10100).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the custom field.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the custom field.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"type": schema.StringAttribute{
				Description: "The type of the custom field (e.g. com.atlassian.jira.plugin.system.customfieldtypes:textfield). Changing this forces recreation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"searcher_key": schema.StringAttribute{
				Description: "The searcher key for the custom field (e.g. com.atlassian.jira.plugin.system.customfieldtypes:textsearcher). Changing this forces recreation.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *customFieldResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *customFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := customFieldCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Type:        plan.Type.ValueString(),
	}
	if !plan.SearcherKey.IsNull() && !plan.SearcherKey.IsUnknown() {
		body.SearcherKey = plan.SearcherKey.ValueString()
	}

	var result customFieldAPIResponse
	err := r.client.Post(ctx, "/rest/api/3/field", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating custom field", err.Error())
		return
	}

	// Server-generated: ID always comes from response.
	plan.ID = types.StringValue(result.ID)

	// SearcherKey: use response value if plan was unknown/null.
	if plan.SearcherKey.IsNull() || plan.SearcherKey.IsUnknown() {
		plan.SearcherKey = types.StringValue(result.SearcherKey)
	}

	// Name, Description, Type are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	field, statusCode, err := r.findFieldByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading custom field", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(field.ID)
	state.Name = types.StringValue(field.Name)
	state.Description = types.StringValue(field.Description)
	state.Type = types.StringValue(field.Schema.Custom)
	state.SearcherKey = types.StringValue(field.SearcherKey)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customFieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan customFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state customFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := customFieldUpdateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	apiPath := fmt.Sprintf("/rest/api/3/field/%s", atlassian.PathEscape(state.ID.ValueString()))
	err := r.client.Put(ctx, apiPath, body, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error updating custom field", err.Error())
		return
	}

	// ID, Type, SearcherKey are carried forward (unchanged on update).
	// Name, Description are preserved from the plan (user intent).

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPath := fmt.Sprintf("/rest/api/3/field/%s", atlassian.PathEscape(state.ID.ValueString()))
	statusCode, err := r.client.DeleteWithStatus(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting custom field", err.Error())
		return
	}

	// 404 means the field was already deleted out-of-band; treat as success.
	if statusCode == http.StatusNotFound {
		return
	}
}

func (r *customFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by field ID (e.g. customfield_10100).
	fieldID := req.ID

	field, statusCode, err := r.findFieldByID(ctx, fieldID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing custom field", err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Custom field not found",
			fmt.Sprintf("No custom field found with ID %q", fieldID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), field.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), field.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("description"), field.Description)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), field.Schema.Custom)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("searcher_key"), field.SearcherKey)...)
}

// findFieldByID fetches all fields and finds the custom field matching the given ID.
// Returns (nil, 404, nil) if no matching custom field is found.
func (r *customFieldResource) findFieldByID(ctx context.Context, fieldID string) (*customFieldAPIResponse, int, error) {
	var fields []customFieldAPIResponse
	err := r.client.Get(ctx, "/rest/api/3/field", &fields)
	if err != nil {
		return nil, 0, err
	}

	for i := range fields {
		if fields[i].Custom && fields[i].ID == fieldID {
			return &fields[i], http.StatusOK, nil
		}
	}

	return nil, http.StatusNotFound, nil
}
