package jira

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &customFieldDataSource{}

// NewCustomFieldDataSource returns a new custom field data source.
func NewCustomFieldDataSource() datasource.DataSource {
	return &customFieldDataSource{}
}

type customFieldDataSource struct {
	client *atlassian.Client
}

type customFieldDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	SearcherKey types.String `tfsdk:"searcher_key"`
}

func (d *customFieldDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_custom_field"
}

func (d *customFieldDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud custom field by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The name of the custom field to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the custom field (e.g. customfield_10100).",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the custom field.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the custom field.",
				Computed:    true,
			},
			"searcher_key": schema.StringAttribute{
				Description: "The searcher key for the custom field.",
				Computed:    true,
			},
		},
	}
}

func (d *customFieldDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *customFieldDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config customFieldDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all fields and find the custom field by name.
	var fields []customFieldAPIResponse
	err := d.client.Get(ctx, "/rest/api/3/field", &fields)
	if err != nil {
		resp.Diagnostics.AddError("Error reading fields", err.Error())
		return
	}

	name := config.Name.ValueString()
	var matched []customFieldAPIResponse
	for _, f := range fields {
		if f.Custom && f.Name == name {
			matched = append(matched, f)
		}
	}

	switch len(matched) {
	case 0:
		resp.Diagnostics.AddError(
			"Custom field not found",
			fmt.Sprintf("No custom field found with name %q", name),
		)
	case 1:
		f := matched[0]
		config.ID = types.StringValue(f.ID)
		config.Description = types.StringValue(f.Description)
		config.Type = types.StringValue(f.Schema.Custom)
		config.SearcherKey = types.StringValue(f.SearcherKey)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	default:
		resp.Diagnostics.AddError(
			"Ambiguous custom field name",
			fmt.Sprintf("Found %d custom fields with name %q; use the ID to import the correct one", len(matched), name),
		)
	}
}
