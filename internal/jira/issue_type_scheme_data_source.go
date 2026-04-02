package jira

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &issueTypeSchemeDataSource{}

// NewIssueTypeSchemeDataSource returns a new issue type scheme data source.
func NewIssueTypeSchemeDataSource() datasource.DataSource {
	return &issueTypeSchemeDataSource{}
}

type issueTypeSchemeDataSource struct {
	client *atlassian.Client
}

type issueTypeSchemeDataSourceModel struct {
	Name               types.String `tfsdk:"name"`
	ID                 types.String `tfsdk:"id"`
	Description        types.String `tfsdk:"description"`
	DefaultIssueTypeID types.String `tfsdk:"default_issue_type_id"`
	IssueTypeIDs       types.List   `tfsdk:"issue_type_ids"`
}

func (d *issueTypeSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_scheme"
}

func (d *issueTypeSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud issue type scheme by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the issue type scheme to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the issue type scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the issue type scheme.",
				Computed:    true,
			},
			"default_issue_type_id": schema.StringAttribute{
				Description: "The ID of the default issue type of the issue type scheme.",
				Computed:    true,
			},
			"issue_type_ids": schema.ListAttribute{
				Description: "An ordered list of issue type IDs in the scheme.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *issueTypeSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *atlassian.Client, got: %T", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *issueTypeSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config issueTypeSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()

	// Fetch all issue type schemes and find by name.
	allValues, err := d.client.GetAllPages(ctx, "/rest/api/3/issuetypescheme")
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type schemes", err.Error())
		return
	}

	var found *issueTypeSchemeAPIResponse
	for _, raw := range allValues {
		var scheme issueTypeSchemeAPIResponse
		if err := json.Unmarshal(raw, &scheme); err != nil {
			resp.Diagnostics.AddError("Error parsing issue type scheme", err.Error())
			return
		}
		if scheme.Name == name {
			schemeCopy := scheme
			found = &schemeCopy
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError(
			"Issue type scheme not found",
			fmt.Sprintf("No issue type scheme found with name %q", name),
		)
		return
	}

	config.ID = types.StringValue(found.ID)
	config.Description = types.StringValue(found.Description)
	if found.DefaultIssueTypeID != "" {
		config.DefaultIssueTypeID = types.StringValue(found.DefaultIssueTypeID)
	} else {
		config.DefaultIssueTypeID = types.StringNull()
	}

	// Fetch the ordered list of issue type IDs for this scheme.
	itemsPath := fmt.Sprintf("/rest/api/3/issuetypescheme/mapping?issueTypeSchemeId=%s", atlassian.QueryEscape(found.ID))
	itemValues, err := d.client.GetAllPages(ctx, itemsPath)
	if err != nil {
		resp.Diagnostics.AddError("Error reading issue type scheme items", err.Error())
		return
	}

	var issueTypeIDs []string
	for _, raw := range itemValues {
		var item issueTypeSchemeItemResponse
		if err := json.Unmarshal(raw, &item); err != nil {
			resp.Diagnostics.AddError("Error parsing issue type scheme item", err.Error())
			return
		}
		if item.IssueTypeSchemeID == found.ID {
			issueTypeIDs = append(issueTypeIDs, item.IssueTypeID)
		}
	}

	listValue, diags := types.ListValueFrom(ctx, types.StringType, issueTypeIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.IssueTypeIDs = listValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
