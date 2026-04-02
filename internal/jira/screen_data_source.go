package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/atlassian"
)

var _ datasource.DataSource = &screenDataSource{}

// NewScreenDataSource returns a new screen data source.
func NewScreenDataSource() datasource.DataSource {
	return &screenDataSource{}
}

type screenDataSource struct {
	client *atlassian.Client
}

type screenDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *screenDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen"
}

func (d *screenDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Jira Cloud screen by name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the screen.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the screen to look up.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the screen.",
				Computed:    true,
			},
		},
	}
}

func (d *screenDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *screenDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config screenDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()

	startAt := 0
	maxResults := 100

	for {
		apiPath := fmt.Sprintf("/rest/api/3/screens?maxResults=%d&startAt=%d", maxResults, startAt)

		var page struct {
			Values  []json.RawMessage `json:"values"`
			IsLast  bool              `json:"isLast"`
			StartAt int               `json:"startAt"`
			Total   int               `json:"total"`
		}

		statusCode, err := d.client.GetWithStatus(ctx, apiPath, &page)
		if err != nil {
			resp.Diagnostics.AddError("Error reading screens", err.Error())
			return
		}
		if statusCode == http.StatusNotFound {
			break
		}

		for _, raw := range page.Values {
			var s screenAPIResponse
			if err := json.Unmarshal(raw, &s); err != nil {
				resp.Diagnostics.AddError("Error parsing screen", err.Error())
				return
			}
			if s.Name == name {
				config.ID = types.StringValue(fmt.Sprintf("%d", s.ID))
				config.Name = types.StringValue(s.Name)
				config.Description = types.StringValue(s.Description)
				resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
				return
			}
		}

		if page.IsLast || len(page.Values) == 0 {
			break
		}

		startAt += len(page.Values)
	}

	resp.Diagnostics.AddError(
		"Screen not found",
		fmt.Sprintf("No screen found with name %q", name),
	)
}
