// Package provider implements the Ghost Inspector Terraform/OpenTofu provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var (
	_ provider.Provider = &GhostInspectorProvider{}
)

// GhostInspectorProvider is the provider implementation.
type GhostInspectorProvider struct {
	version string
}

// GhostInspectorProviderModel describes the provider configuration.
type GhostInspectorProviderModel struct {
	APIKey types.String `tfsdk:"api_key"`
}

// New returns a provider factory, for providerserver.Serve and tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &GhostInspectorProvider{version: version}
	}
}

func (p *GhostInspectorProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ghostinspector"
	resp.Version = p.version
}

func (p *GhostInspectorProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Ghost Inspector suites, tests, steps, and suite variables as code.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Ghost Inspector API key. May also be set with the GHOSTINSPECTOR_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *GhostInspectorProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config GhostInspectorProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := os.Getenv("GHOSTINSPECTOR_API_KEY")
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() && config.APIKey.ValueString() != "" {
		apiKey = config.APIKey.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing Ghost Inspector API key",
			"Set api_key in the provider configuration or the GHOSTINSPECTOR_API_KEY environment variable.",
		)
		return
	}

	client := gi.NewClient(apiKey)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *GhostInspectorProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFolderResource,
		NewSuiteResource,
		NewSuiteVariablesResource,
		NewTestResource,
	}
}

func (p *GhostInspectorProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewFolderDataSource,
	}
}
