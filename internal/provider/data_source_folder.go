package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var _ datasource.DataSource = &FolderDataSource{}

// FolderDataSource looks up a folder by ID or name.
type FolderDataSource struct {
	client *gi.Client
}

// FolderDataSourceModel is the data source model.
type FolderDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Organization types.String `tfsdk:"organization"`
}

// NewFolderDataSource creates the data source.
func NewFolderDataSource() datasource.DataSource { return &FolderDataSource{} }

func (d *FolderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (d *FolderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a Ghost Inspector folder by id or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Folder ID. Exactly one of id or name must be set.",
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Folder name. Exactly one of id or name must be set.",
			},
			"organization": schema.StringAttribute{
				Computed:    true,
				Description: "Owning organization ID.",
			},
		},
	}
}

func (d *FolderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*gi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *gi.Client, got %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *FolderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FolderDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	byID := !config.ID.IsNull() && config.ID.ValueString() != ""
	byName := !config.Name.IsNull() && config.Name.ValueString() != ""
	if byID == byName {
		resp.Diagnostics.AddError("Invalid lookup", "Exactly one of id or name must be set.")
		return
	}

	folders, err := d.client.ListFolders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Folder lookup failed", err.Error())
		return
	}
	for _, f := range folders {
		if (byID && f.ID == config.ID.ValueString()) || (byName && f.Name == config.Name.ValueString()) {
			config.ID = types.StringValue(f.ID)
			config.Name = types.StringValue(f.Name)
			if org := f.OrgID(); org != "" {
				config.Organization = types.StringValue(org)
			} else {
				config.Organization = types.StringNull()
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError("Folder not found", "No folder matched the given id or name.")
}
