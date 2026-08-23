package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var (
	_ resource.Resource                = &FolderResource{}
	_ resource.ResourceWithImportState = &FolderResource{}
)

// FolderResource manages a Ghost Inspector folder. Creation adopts an
// existing folder with the same name instead of failing.
type FolderResource struct {
	client *gi.Client
}

// FolderResourceModel is the state model.
type FolderResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Organization types.String `tfsdk:"organization"`
}

// NewFolderResource creates the resource.
func NewFolderResource() resource.Resource { return &FolderResource{} }

func (r *FolderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (r *FolderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Ghost Inspector folder. If a folder with the same name already exists it is adopted into state. The Ghost Inspector API has no folder deletion endpoint, so destroying this resource only removes it from state and leaves the folder in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Ghost Inspector folder ID.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Folder name. Renamed in place.",
			},
			"organization": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Organization that owns the folder. When null, creation tries the organizations visible to the API key until one accepts the write. Keys scoped to a single organization should set this explicitly.",
			},
		},
	}
}

func (r *FolderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*gi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *gi.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *FolderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FolderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := plan.Name.ValueString()

	existing, err := r.client.FindFolderByName(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Folder lookup failed", err.Error())
		return
	}

	var folder *gi.Folder
	if existing != nil {
		folder = existing
		resp.Diagnostics.AddWarning(
			"Adopted existing folder",
			fmt.Sprintf("A folder named %q already existed (%s) and was adopted into state.", name, existing.ID),
		)
	} else {
		// The caller's API key may see several organizations and not all of
		// them accept writes, so try the configured organization first, then
		// every organization visible through existing folders and suites, and
		// finally no organization at all (API default).
		candidates := []string{}
		seen := map[string]bool{}
		add := func(org string) {
			if !seen[org] {
				seen[org] = true
				candidates = append(candidates, org)
			}
		}
		if !plan.Organization.IsNull() && !plan.Organization.IsUnknown() && plan.Organization.ValueString() != "" {
			add(plan.Organization.ValueString())
		}
		if orgs, orgErr := r.client.OrganizationIDs(ctx); orgErr == nil {
			for _, org := range orgs {
				add(org)
			}
		}
		add("")

		var lastErr error
		for _, org := range candidates {
			folder, lastErr = r.client.CreateFolder(ctx, name, org)
			if lastErr == nil {
				break
			}
		}
		if folder == nil {
			resp.Diagnostics.AddError("Folder creation failed", fmt.Sprintf("tried %d organization(s), last error: %v", len(candidates), lastErr))
			return
		}
	}

	plan.ID = types.StringValue(folder.ID)
	plan.Name = types.StringValue(folder.Name)
	if org := folder.OrgID(); org != "" {
		plan.Organization = types.StringValue(org)
	} else {
		plan.Organization = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FolderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FolderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	folders, err := r.client.ListFolders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Folder lookup failed", err.Error())
		return
	}
	for _, f := range folders {
		if f.ID == state.ID.ValueString() {
			state.Name = types.StringValue(f.Name)
			if org := f.OrgID(); org != "" {
				state.Organization = types.StringValue(org)
			} else {
				state.Organization = types.StringNull()
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *FolderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FolderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UpdateFolder(ctx, plan.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Folder rename failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FolderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FolderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"Folder left in place",
		fmt.Sprintf("The Ghost Inspector API does not support folder deletion, so folder %q (%s) was only removed from state and still exists in Ghost Inspector. Delete it in the UI if it is no longer needed.", state.Name.ValueString(), state.ID.ValueString()),
	)
}

func (r *FolderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
