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
	_ resource.Resource                = &projectIssueTypeSchemeResource{}
	_ resource.ResourceWithImportState = &projectIssueTypeSchemeResource{}
)

// NewProjectIssueTypeSchemeResource returns a new project issue type scheme association resource.
func NewProjectIssueTypeSchemeResource() resource.Resource {
	return &projectIssueTypeSchemeResource{}
}

type projectIssueTypeSchemeResource struct {
	client *atlassian.Client
}

type projectIssueTypeSchemeResourceModel struct {
	ProjectID         types.String `tfsdk:"project_id"`
	IssueTypeSchemeID types.String `tfsdk:"issue_type_scheme_id"`
}

// projectIssueTypeSchemeListResponse represents the paginated GET response for
// /rest/api/3/issuetypescheme/project.
type projectIssueTypeSchemeListResponse struct {
	IssueTypeScheme issueTypeSchemeAPIResponse `json:"issueTypeScheme"`
	ProjectIDs      []string                   `json:"projectIds"`
}

func (r *projectIssueTypeSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_issue_type_scheme"
}

func (r *projectIssueTypeSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Associates a Jira Cloud project with an issue type scheme.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Description: "The ID of the project. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"issue_type_scheme_id": schema.StringAttribute{
				Description: "The ID of the issue type scheme to assign. Changing this forces recreation of the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *projectIssueTypeSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectIssueTypeSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectIssueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]string{
		"issueTypeSchemeId": plan.IssueTypeSchemeID.ValueString(),
		"projectId":         plan.ProjectID.ValueString(),
	}

	err := r.client.Put(ctx, "/rest/api/3/issuetypescheme/project", body, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error assigning issue type scheme to project", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectIssueTypeSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectIssueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()
	apiPath := fmt.Sprintf("/rest/api/3/issuetypescheme/project?projectId=%s", atlassian.QueryEscape(projectID))

	allValues, err := r.client.GetAllPages(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project issue type scheme", err.Error())
		return
	}

	for _, raw := range allValues {
		var entry projectIssueTypeSchemeListResponse
		if err := json.Unmarshal(raw, &entry); err != nil {
			resp.Diagnostics.AddError("Error parsing project issue type scheme response", err.Error())
			return
		}
		for _, pid := range entry.ProjectIDs {
			if pid == projectID {
				state.IssueTypeSchemeID = types.StringValue(entry.IssueTypeScheme.ID)
				resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
				return
			}
		}
	}

	// Project/association not found — remove from state.
	resp.State.RemoveResource(ctx)
}

func (r *projectIssueTypeSchemeResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both fields have RequiresReplace, so Update is never called.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Changing project_id or issue_type_scheme_id requires replacing the resource.",
	)
}

func (r *projectIssueTypeSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectIssueTypeSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The Jira API does not support removing an issue type scheme association.
	// On destroy, revert the project to the default issue type scheme (ID 10000).
	const defaultIssueTypeSchemeID = "10000"

	body := map[string]string{
		"issueTypeSchemeId": defaultIssueTypeSchemeID,
		"projectId":         state.ProjectID.ValueString(),
	}

	err := r.client.Put(ctx, "/rest/api/3/issuetypescheme/project", body, nil)
	if err != nil {
		// Tolerate 404: project may have been deleted out-of-band.
		// Treat as a non-fatal warning so Terraform can still clean up state.
		resp.Diagnostics.AddWarning(
			"Could not revert issue type scheme to default",
			fmt.Sprintf("Error reverting project %q to the default issue type scheme: %s. "+
				"The project may have been deleted or the scheme association was already removed.",
				state.ProjectID.ValueString(), err.Error()),
		)
	}
}

func (r *projectIssueTypeSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID := req.ID

	apiPath := fmt.Sprintf("/rest/api/3/issuetypescheme/project?projectId=%s", atlassian.QueryEscape(projectID))

	allValues, err := r.client.GetAllPages(ctx, apiPath)
	if err != nil {
		resp.Diagnostics.AddError("Error importing project issue type scheme", err.Error())
		return
	}

	for _, raw := range allValues {
		var entry projectIssueTypeSchemeListResponse
		if err := json.Unmarshal(raw, &entry); err != nil {
			resp.Diagnostics.AddError("Error parsing project issue type scheme response", err.Error())
			return
		}
		for _, pid := range entry.ProjectIDs {
			if pid == projectID {
				resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
				resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("issue_type_scheme_id"), entry.IssueTypeScheme.ID)...)
				return
			}
		}
	}

	resp.Diagnostics.AddError(
		"Project issue type scheme not found",
		fmt.Sprintf("No issue type scheme association found for project ID %q", projectID),
	)
}

