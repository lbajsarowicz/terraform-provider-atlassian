package jira

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var (
	_ resource.Resource                = &projectIssueTypeScreenSchemeResource{}
	_ resource.ResourceWithImportState = &projectIssueTypeScreenSchemeResource{}
)

// NewProjectIssueTypeScreenSchemeResource returns a new project issue type screen scheme association resource.
func NewProjectIssueTypeScreenSchemeResource() resource.Resource {
	return &projectIssueTypeScreenSchemeResource{}
}

type projectIssueTypeScreenSchemeResource struct {
	client *atlassian.Client
}

type projectIssueTypeScreenSchemeResourceModel struct {
	ProjectID               types.String `tfsdk:"project_id"`
	IssueTypeScreenSchemeID types.String `tfsdk:"issue_type_screen_scheme_id"`
}

// projectIssueTypeScreenSchemeListEntry represents a single entry in the
// GET /rest/api/3/issuetypescreenscheme/project response values array.
type projectIssueTypeScreenSchemeListEntry struct {
	IssueTypeScreenScheme issueTypeScreenSchemeAPIResponse `json:"issueTypeScreenScheme"`
	ProjectIDs            []string                         `json:"projectIds"`
}

func (r *projectIssueTypeScreenSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_issue_type_screen_scheme"
}

func (r *projectIssueTypeScreenSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Associates a Jira Cloud project with an issue type screen scheme.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Description: "The ID of the project. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"issue_type_screen_scheme_id": schema.StringAttribute{
				Description: "The ID of the issue type screen scheme. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *projectIssueTypeScreenSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectIssueTypeScreenSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectIssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]string{
		"issueTypeScreenSchemeId": plan.IssueTypeScreenSchemeID.ValueString(),
		"projectId":               plan.ProjectID.ValueString(),
	}

	err := r.client.Put(ctx, "/rest/api/3/issuetypescreenscheme/project", body, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error assigning issue type screen scheme to project", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectIssueTypeScreenSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectIssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()
	apiPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/project?projectId=%s", atlassian.QueryEscape(projectID))

	allValues, err := r.client.GetAllPages(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project issue type screen scheme", err.Error())
		return
	}

	for _, raw := range allValues {
		var entry projectIssueTypeScreenSchemeListEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			resp.Diagnostics.AddError("Error parsing project issue type screen scheme response", err.Error())
			return
		}
		if entry.IssueTypeScreenScheme.ID == state.IssueTypeScreenSchemeID.ValueString() {
			state.IssueTypeScreenSchemeID = types.StringValue(entry.IssueTypeScreenScheme.ID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Association not found — remove from state.
	resp.State.RemoveResource(ctx)
}

func (r *projectIssueTypeScreenSchemeResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both fields have RequiresReplace, so Update is never called.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Changing project_id or issue_type_screen_scheme_id requires replacing the resource.",
	)
}

func (r *projectIssueTypeScreenSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectIssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The Jira API does not support removing an issue type screen scheme association.
	// On destroy, revert the project to the default issue type screen scheme (ID 1).
	const defaultIssueTypeScreenSchemeID = "1"

	body := map[string]string{
		"issueTypeScreenSchemeId": defaultIssueTypeScreenSchemeID,
		"projectId":               state.ProjectID.ValueString(),
	}

	err := r.client.Put(ctx, "/rest/api/3/issuetypescreenscheme/project", body, nil)
	if err != nil {
		// Tolerate failure (e.g. project deleted out-of-band); emit a warning so
		// Terraform can still clean up state.
		resp.Diagnostics.AddWarning(
			"Could not revert issue type screen scheme to default",
			fmt.Sprintf("Error reverting project %q to the default issue type screen scheme: %s. "+
				"The project may have been deleted or the scheme association was already removed.",
				state.ProjectID.ValueString(), err.Error()),
		)
	}
}

func (r *projectIssueTypeScreenSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: {projectId}
	// Resolve the current scheme from the API so state reflects reality.
	projectID := req.ID

	apiPath := fmt.Sprintf("/rest/api/3/issuetypescreenscheme/project?projectId=%s", atlassian.QueryEscape(projectID))

	allValues, err := r.client.GetAllPages(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error importing project issue type screen scheme", err.Error())
		return
	}

	if len(allValues) == 0 {
		resp.Diagnostics.AddError(
			"Project issue type screen scheme not found",
			fmt.Sprintf("No issue type screen scheme association found for project ID %q", projectID),
		)
		return
	}

	var entry projectIssueTypeScreenSchemeListEntry
	if err := json.Unmarshal(allValues[0], &entry); err != nil {
		resp.Diagnostics.AddError("Error parsing project issue type screen scheme response", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("issue_type_screen_scheme_id"), entry.IssueTypeScreenScheme.ID)...)
}
