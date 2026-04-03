package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// spaceCreateV2Response is the response from POST /wiki/api/v2/spaces.
// Description uses a nested format: description.plain.value
type spaceCreateV2Response struct {
	ID  json.Number `json:"id"`
	Key string      `json:"key"`
}

// spaceV1Response is the response from GET /wiki/rest/api/space/{key}?expand=description.plain.
// The v1 API returns description correctly; v2 GET returns an empty description object.
type spaceV1Response struct {
	ID          json.Number `json:"id"`
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Description *struct {
		Plain *struct {
			Value string `json:"value"`
		} `json:"plain"`
	} `json:"description"`
}

// spaceCreateRequest is the body for POST /wiki/api/v2/spaces.
// The description format in the POST body is flat: {value, representation}.
type spaceCreateRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description struct {
		Value          string `json:"value"`
		Representation string `json:"representation"`
	} `json:"description"`
}

// spaceUpdateRequest is the body for PUT /wiki/rest/api/space/{key} (v1).
// The v1 update uses nested description: description.plain.{value, representation}.
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

	var createResult spaceCreateV2Response
	if err := r.client.Post(ctx, "/wiki/api/v2/spaces", body, &createResult); err != nil {
		resp.Diagnostics.AddError("Error creating Confluence space", err.Error())
		return
	}

	// The v2 GET response returns an empty description object; use v1 to read
	// the actual stored state including description.
	space, statusCode, err := r.readSpaceV1(ctx, createResult.Key)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Confluence space after create", err.Error())
		return
	}
	if statusCode == http.StatusNotFound {
		resp.Diagnostics.AddError("Error reading Confluence space after create", "space not found immediately after creation")
		return
	}

	plan.ID = types.StringValue(space.ID.String())
	plan.Key = types.StringValue(space.Key)
	plan.Name = types.StringValue(space.Name)
	plan.Description = types.StringValue(descriptionValue(space))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state spaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := state.Key.ValueString()

	// After import, key may be empty; look it up from the space ID via v2.
	if key == "" {
		var v2result struct {
			Key string `json:"key"`
		}
		apiPath := fmt.Sprintf("/wiki/api/v2/spaces/%s", atlassian.PathEscape(state.ID.ValueString()))
		statusCode, err := r.client.GetWithStatus(ctx, apiPath, &v2result)
		if err != nil {
			resp.Diagnostics.AddError("Error reading Confluence space", err.Error())
			return
		}
		if statusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		key = v2result.Key
	}

	space, statusCode, err := r.readSpaceV1(ctx, key)
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
	state.Description = types.StringValue(descriptionValue(space))

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

	// Re-read via v1 to populate fresh state.
	space, statusCode, err := r.readSpaceV1(ctx, key)
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
	plan.Description = types.StringValue(descriptionValue(space))

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
		// The Confluence API returns longtask paths without the /wiki context prefix
		// (e.g. "/rest/api/longtask/123" instead of "/wiki/rest/api/longtask/123").
		taskPath := taskRef.Links.Status
		if strings.HasPrefix(taskPath, "/rest/api/longtask") {
			taskPath = "/wiki" + taskPath
		}
		if err := r.client.PollLongTask(ctx, taskPath); err != nil {
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

// readSpaceV1 fetches a Confluence space by key using the v1 API with description expansion.
// Returns (nil, 404, nil) if the space is not found.
func (r *spaceResource) readSpaceV1(ctx context.Context, key string) (*spaceV1Response, int, error) {
	apiPath := fmt.Sprintf("/wiki/rest/api/space/%s?expand=description.plain", atlassian.PathEscape(key))
	var result spaceV1Response
	statusCode, err := r.client.GetWithStatus(ctx, apiPath, &result)
	if err != nil {
		return nil, statusCode, err
	}
	if statusCode == http.StatusNotFound {
		return nil, http.StatusNotFound, nil
	}
	return &result, statusCode, nil
}

// descriptionValue extracts the plain description value from a v1 space response.
func descriptionValue(s *spaceV1Response) string {
	if s.Description != nil && s.Description.Plain != nil {
		return s.Description.Plain.Value
	}
	return ""
}
