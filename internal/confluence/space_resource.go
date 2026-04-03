package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	_ resource.Resource                = &spaceResource{}
	_ resource.ResourceWithImportState = &spaceResource{}
)

// NewSpaceResource returns a new Confluence space resource.
func NewSpaceResource() resource.Resource {
	return &spaceResource{}
}

type spaceResource struct {
	client *atlassian.Client
}

type spaceResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// spaceV2Response is the Confluence v2 API space representation.
type spaceV2Response struct {
	ID          json.Number `json:"id"`
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Description *struct {
		Value string `json:"value"`
	} `json:"description"`
}

// spaceCreateRequest is the body for POST /wiki/api/v2/spaces.
type spaceCreateRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description struct {
		Value          string `json:"value"`
		Representation string `json:"representation"`
	} `json:"description"`
}

// spaceUpdateRequest is the body for PUT /wiki/rest/api/space/{key} (v1).
type spaceUpdateRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description struct {
		Plain struct {
			Value          string `json:"value"`
			Representation string `json:"representation"`
		} `json:"plain"`
	} `json:"description"`
}

// spaceLongTaskRef is the 202 response from DELETE /wiki/rest/api/space/{key}.
type spaceLongTaskRef struct {
	Links struct {
		Status string `json:"status"`
	} `json:"links"`
}

func (r *spaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space"
}

func (r *spaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Confluence Cloud space.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The numeric ID of the space.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Description: "The space key (uppercase letters, e.g. MYSPACE). Changing this forces recreation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the space.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The plain-text description of the space.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
		},
	}
}

func (r *spaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *spaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan spaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := spaceCreateRequest{
		Key:  plan.Key.ValueString(),
		Name: plan.Name.ValueString(),
	}
	body.Description.Value = plan.Description.ValueString()
	body.Description.Representation = "plain"

	var result spaceV2Response
	if err := r.client.Post(ctx, "/wiki/api/v2/spaces", body, &result); err != nil {
		resp.Diagnostics.AddError("Error creating Confluence space", err.Error())
		return
	}

	plan.ID = types.StringValue(result.ID.String())
	plan.Key = types.StringValue(result.Key)
	plan.Name = types.StringValue(result.Name)
	if result.Description != nil {
		plan.Description = types.StringValue(result.Description.Value)
	} else {
		plan.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state spaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	space, statusCode, err := r.readSpaceByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Confluence space", err.Error())
		return
	}
	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(space.ID.String())
	state.Key = types.StringValue(space.Key)
	state.Name = types.StringValue(space.Name)
	if space.Description != nil {
		state.Description = types.StringValue(space.Description.Value)
	} else {
		state.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *spaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan spaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state spaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := state.Key.ValueString()
	body := spaceUpdateRequest{
		Key:  key,
		Name: plan.Name.ValueString(),
	}
	body.Description.Plain.Value = plan.Description.ValueString()
	body.Description.Plain.Representation = "plain"

	apiPath := fmt.Sprintf("/wiki/rest/api/space/%s", atlassian.PathEscape(key))
	if err := r.client.Put(ctx, apiPath, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating Confluence space", err.Error())
		return
	}

	// Re-read via v2 to populate fresh state
	space, statusCode, err := r.readSpaceByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Confluence space after update", err.Error())
		return
	}
	if statusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	plan.ID = state.ID
	plan.Key = types.StringValue(space.Key)
	plan.Name = types.StringValue(space.Name)
	if space.Description != nil {
		plan.Description = types.StringValue(space.Description.Value)
	} else {
		plan.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state spaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := state.Key.ValueString()
	apiPath := fmt.Sprintf("/wiki/rest/api/space/%s", atlassian.PathEscape(key))

	httpResp, err := r.client.Do(ctx, "DELETE", apiPath, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting Confluence space", err.Error())
		return
	}
	defer httpResp.Body.Close()

	switch httpResp.StatusCode {
	case http.StatusNotFound:
		// Already deleted out-of-band; treat as success.
		return
	case http.StatusNoContent, http.StatusOK:
		return
	case http.StatusAccepted:
		// Async deletion: parse long task reference and poll.
		var taskRef spaceLongTaskRef
		if err := json.NewDecoder(httpResp.Body).Decode(&taskRef); err != nil {
			resp.Diagnostics.AddError("Error parsing Confluence space delete response", err.Error())
			return
		}
		if taskRef.Links.Status == "" {
			// No task link; assume deletion succeeded.
			return
		}
		if err := r.client.PollLongTask(ctx, taskRef.Links.Status); err != nil {
			resp.Diagnostics.AddError("Error waiting for Confluence space deletion", err.Error())
			return
		}
	default:
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError(
			"Error deleting Confluence space",
			fmt.Sprintf("DELETE %s: unexpected status %d: %s", apiPath, httpResp.StatusCode, string(bodyBytes)),
		)
	}
}

func (r *spaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readSpaceByID fetches a Confluence space by its numeric ID via the v2 API.
// Returns (nil, 404, nil) if the space is not found.
func (r *spaceResource) readSpaceByID(ctx context.Context, spaceID string) (*spaceV2Response, int, error) {
	apiPath := fmt.Sprintf("/wiki/api/v2/spaces/%s", atlassian.PathEscape(spaceID))

	var result spaceV2Response
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		return nil, statusCode, err
	}
	if statusCode == http.StatusNotFound {
		return nil, http.StatusNotFound, nil
	}
	return &result, statusCode, nil
}
