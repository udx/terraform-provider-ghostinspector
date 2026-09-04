package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// settingsModel holds the test/suite settings shared by both resources.
// Attribute names mirror the Ghost Inspector API (camelCase) so payloads and
// documentation line up one-to-one. Every attribute is Optional+Computed:
// a null config value means "not managed by this configuration" and simply
// mirrors whatever the API reports (inherit/default semantics).
type settingsModel struct {
	Browser                    types.String  `tfsdk:"browser"`
	Region                     types.String  `tfsdk:"region"`
	UserAgent                  types.String  `tfsdk:"user_agent"`
	Geolocation                types.String  `tfsdk:"geolocation"`
	MaxWaitDelay               types.Int64   `tfsdk:"max_wait_delay"`
	MaxAjaxDelay               types.Int64   `tfsdk:"max_ajax_delay"`
	GlobalStepDelay            types.Int64   `tfsdk:"global_step_delay"`
	FinalDelay                 types.Int64   `tfsdk:"final_delay"`
	AutoRetry                  types.Bool    `tfsdk:"auto_retry"`
	ScreenshotCompareEnabled   types.Bool    `tfsdk:"screenshot_compare_enabled"`
	ScreenshotCompareThreshold types.Float64 `tfsdk:"screenshot_compare_threshold"`
	FailOnJavaScriptError      types.Bool    `tfsdk:"fail_on_javascript_error"`
}

func settingsAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"browser": schema.StringAttribute{
			Optional: true, Computed: true,
			Description:   "Browser to run with (for example chrome, firefox). Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"region": schema.StringAttribute{
			Optional: true, Computed: true,
			Description:   "Execution region (for example us-east-1). Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"user_agent": schema.StringAttribute{
			Optional: true, Computed: true,
			Description:   "Custom user agent string. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"geolocation": schema.StringAttribute{
			Optional: true, Computed: true,
			Description:   "Geolocation override. Write-only: the Ghost Inspector API discards it on update and never returns it on read, so the configured value is kept in state. Null leaves it unmanaged.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"max_wait_delay": schema.Int64Attribute{
			Optional: true, Computed: true,
			Description:   "Maximum wait (ms) for elements and assertions. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		},
		"max_ajax_delay": schema.Int64Attribute{
			Optional: true, Computed: true,
			Description:   "Maximum wait (ms) for AJAX calls. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		},
		"global_step_delay": schema.Int64Attribute{
			Optional: true, Computed: true,
			Description:   "Delay (ms) inserted between every step. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		},
		"final_delay": schema.Int64Attribute{
			Optional: true, Computed: true,
			Description:   "Delay (ms) at the end of the test before finishing. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		},
		"auto_retry": schema.BoolAttribute{
			Optional: true, Computed: true,
			Description:   "Retry the test once automatically on failure. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		},
		"screenshot_compare_enabled": schema.BoolAttribute{
			Optional: true, Computed: true,
			Description:   "Enable screenshot comparison against the baseline. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		},
		"screenshot_compare_threshold": schema.Float64Attribute{
			Optional: true, Computed: true,
			Description:   "Allowed screenshot difference ratio (for example 0.1 for 10%). Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
		},
		"fail_on_javascript_error": schema.BoolAttribute{
			Optional: true, Computed: true,
			Description:   "Fail the test when a JavaScript error is detected. Null leaves the API value unmanaged.",
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		},
	}
}

// apiFields renders the non-null settings as an API payload fragment.
func (m *settingsModel) apiFields() map[string]interface{} {
	out := map[string]interface{}{}
	if !m.Browser.IsNull() && !m.Browser.IsUnknown() {
		out["browser"] = m.Browser.ValueString()
	}
	if !m.Region.IsNull() && !m.Region.IsUnknown() {
		out["region"] = m.Region.ValueString()
	}
	if !m.UserAgent.IsNull() && !m.UserAgent.IsUnknown() {
		out["userAgent"] = m.UserAgent.ValueString()
	}
	if !m.Geolocation.IsNull() && !m.Geolocation.IsUnknown() {
		out["geolocation"] = m.Geolocation.ValueString()
	}
	if !m.MaxWaitDelay.IsNull() && !m.MaxWaitDelay.IsUnknown() {
		out["maxWaitDelay"] = m.MaxWaitDelay.ValueInt64()
	}
	if !m.MaxAjaxDelay.IsNull() && !m.MaxAjaxDelay.IsUnknown() {
		out["maxAjaxDelay"] = m.MaxAjaxDelay.ValueInt64()
	}
	if !m.GlobalStepDelay.IsNull() && !m.GlobalStepDelay.IsUnknown() {
		out["globalStepDelay"] = m.GlobalStepDelay.ValueInt64()
	}
	if !m.FinalDelay.IsNull() && !m.FinalDelay.IsUnknown() {
		out["finalDelay"] = m.FinalDelay.ValueInt64()
	}
	if !m.AutoRetry.IsNull() && !m.AutoRetry.IsUnknown() {
		out["autoRetry"] = m.AutoRetry.ValueBool()
	}
	if !m.ScreenshotCompareEnabled.IsNull() && !m.ScreenshotCompareEnabled.IsUnknown() {
		out["screenshotCompareEnabled"] = m.ScreenshotCompareEnabled.ValueBool()
	}
	if !m.ScreenshotCompareThreshold.IsNull() && !m.ScreenshotCompareThreshold.IsUnknown() {
		out["screenshotCompareThreshold"] = m.ScreenshotCompareThreshold.ValueFloat64()
	}
	if !m.FailOnJavaScriptError.IsNull() && !m.FailOnJavaScriptError.IsUnknown() {
		out["failOnJavaScriptError"] = m.FailOnJavaScriptError.ValueBool()
	}
	return out
}

// mergeAttrs copies attributes from b that are not already present in a.
func mergeAttrs(a, b map[string]schema.Attribute) map[string]schema.Attribute {
	for k, v := range b {
		if _, ok := a[k]; !ok {
			a[k] = v
		}
	}
	return a
}
