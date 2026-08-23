package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var (
	_ resource.Resource                = &SuiteResource{}
	_ resource.ResourceWithImportState = &SuiteResource{}
)

// SuiteResource manages a Ghost Inspector suite and its default test
// settings. Creation adopts an existing suite with the same name in the
// target folder. Suite variables are managed by the separate
// ghostinspector_suite_variables resource.
type SuiteResource struct {
	client *gi.Client
}

type scheduleModel struct {
	Enabled  types.Bool   `tfsdk:"enabled"`
	Interval types.String `tfsdk:"interval"`
	Time     types.String `tfsdk:"time"`
}

// SuiteResourceModel is the state model.
type SuiteResourceModel struct {
	ID          types.String `tfsdk:"id"`
	SuiteID     types.String `tfsdk:"suite_id"`
	Name        types.String `tfsdk:"name"`
	FolderID    types.String `tfsdk:"folder_id"`
	Description types.String `tfsdk:"description"`
	Schedule    types.Object `tfsdk:"schedule"`
	settingsModel
}

// NewSuiteResource creates the resource.
func NewSuiteResource() resource.Resource { return &SuiteResource{} }

func (r *SuiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_suite"
}

func (r *SuiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Ghost Inspector suite and its default test settings. If a suite with the same name already exists in the folder it is adopted into state. A null settings attribute leaves the API-side value unmanaged.",
		Attributes: mergeAttrs(settingsAttributes(), map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Ghost Inspector suite ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Suite name.",
			},
			"suite_id": schema.StringAttribute{
				Optional:    true,
				Description: "Existing suite ID to adopt on create. When set and the suite exists, it is adopted into state instead of created. When set and the suite does not exist, creation fails rather than creating a different suite.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"folder_id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Folder the suite belongs to. Suites without a folder are supported.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"description": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Suite description. Write-only: the Ghost Inspector API discards it on update and never returns it on read, so the configured value is kept in state. Null leaves it unmanaged.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"schedule": schema.SingleNestedAttribute{
				Optional: true, Computed: true,
				Description: "Suite run schedule. Write-only: the Ghost Inspector API discards it on update and never returns it on read, so the configured value is kept in state. Null leaves it unmanaged.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Optional: true, Computed: true,
						Description: "Whether the schedule is enabled.",
					},
					"interval": schema.StringAttribute{
						Optional: true, Computed: true,
						Description: "Schedule interval, such as daily.",
					},
					"time": schema.StringAttribute{
						Optional: true, Computed: true,
						Description: "Schedule time, such as 08:00.",
					},
				},
			},
		}),
	}
}

func (r *SuiteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func scheduleObject(enabled, hasSchedule bool, s *gi.Schedule) types.Object {
	if !hasSchedule || s == nil {
		return types.ObjectNull(scheduleAttrTypes())
	}
	return types.ObjectValueMust(scheduleAttrTypes(), map[string]attr.Value{
		"enabled":  types.BoolValue(s.Enabled),
		"interval": stringOrNull(s.Interval),
		"time":     stringOrNull(s.Time),
	})
}

func scheduleAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled":  types.BoolType,
		"interval": types.StringType,
		"time":     types.StringType,
	}
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func (m *SuiteResourceModel) fromAPI(s *gi.Suite) {
	m.ID = types.StringValue(s.ID)
	m.Name = types.StringValue(s.Name)
	m.FolderID = stringOrNull(s.Folder)
	// The suite update API silently discards description, so it never comes
	// back on read. Keep the configured value in state (write-only).
	if s.Description != "" {
		m.Description = types.StringValue(s.Description)
	} else if m.Description.IsUnknown() {
		m.Description = types.StringNull()
	}
	m.Browser = stringOrNull(ptrStr(s.Browser))
	m.Region = stringOrNull(ptrStr(s.Region))
	m.UserAgent = stringOrNull(ptrStr(s.UserAgent))
	m.Geolocation = stringOrNull(ptrStr(s.Geolocation))
	m.MaxWaitDelay = intOrNull(s.MaxWaitDelay)
	m.MaxAjaxDelay = intOrNull(s.MaxAjaxDelay)
	m.GlobalStepDelay = intOrNull(s.GlobalStepDelay)
	m.FinalDelay = intOrNull(s.FinalDelay)
	m.AutoRetry = boolOrNull(s.AutoRetry)
	m.ScreenshotCompareEnabled = boolOrNull(s.ScreenshotCompareEnabled)
	m.ScreenshotCompareThreshold = floatOrNull(s.ScreenshotCompareThreshold)
	m.FailOnJavaScriptError = boolOrNull(s.FailOnJavaScriptError)
	// The suite update API silently discards schedule, so it never comes back
	// on read. Keep the configured value in state (write-only).
	if s.Schedule != nil {
		m.Schedule = scheduleObject(true, true, s.Schedule)
	} else if m.Schedule.IsUnknown() {
		m.Schedule = types.ObjectNull(scheduleAttrTypes())
	}
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intOrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func boolOrNull(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

func floatOrNull(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}

func (r *SuiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SuiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := plan.Name.ValueString()
	folderID := ""
	if !plan.FolderID.IsNull() && !plan.FolderID.IsUnknown() {
		folderID = plan.FolderID.ValueString()
	}

	// Adopt-by-ID fast path: the configuration pins the exact suite to manage.
	if !plan.SuiteID.IsNull() && !plan.SuiteID.IsUnknown() && plan.SuiteID.ValueString() != "" {
		targetID := plan.SuiteID.ValueString()
		suite, err := r.client.GetSuite(ctx, targetID)
		if gi.NotFound(err) {
			resp.Diagnostics.AddError(
				"Suite not found",
				fmt.Sprintf("suite_id %q was configured for adoption but no such suite exists. Creation is refused so a differently-named suite is not created by accident.", targetID),
			)
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Suite lookup failed", err.Error())
			return
		}
		resp.Diagnostics.AddWarning(
			"Adopted existing suite",
			fmt.Sprintf("Suite %q (%s) was adopted into state by suite_id.", suite.Name, suite.ID),
		)
		if err := r.pushSettings(ctx, suite.ID, &plan); err != nil {
			resp.Diagnostics.AddError("Suite settings sync failed", err.Error())
			return
		}
		if err := r.refresh(ctx, suite.ID, &plan); err != nil {
			resp.Diagnostics.AddError("Suite read failed", err.Error())
			return
		}
		plan.SuiteID = types.StringValue(suite.ID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	var suite *gi.Suite
	var candidates []gi.Suite
	var err error
	if folderID != "" {
		candidates, err = r.client.ListFolderSuites(ctx, folderID)
	} else {
		candidates, err = r.client.ListSuites(ctx)
	}
	if err != nil {
		resp.Diagnostics.AddError("Suite lookup failed", err.Error())
		return
	}
	for _, s := range candidates {
		if s.Name == name && (folderID != "" || s.Folder == "") {
			suite = &s
			break
		}
	}

	if suite != nil {
		resp.Diagnostics.AddWarning(
			"Adopted existing suite",
			fmt.Sprintf("A suite named %q already existed (%s) and was adopted into state.", name, suite.ID),
		)
	} else {
		// The Ghost Inspector API requires an explicit organization on suite
		// creation; deriving it from the folder fails for keys that can see
		// several organizations. Resolve it from the folder, or fall back to
		// trying every visible organization.
		var orgCandidates []string
		if folderID != "" {
			if folders, ferr := r.client.ListFolders(ctx); ferr == nil {
				for _, f := range folders {
					if f.ID == folderID && f.OrgID() != "" {
						orgCandidates = append(orgCandidates, f.OrgID())
						break
					}
				}
			}
		} else if orgs, oerr := r.client.OrganizationIDs(ctx); oerr == nil {
			orgCandidates = append(orgCandidates, orgs...)
		}
		orgCandidates = append(orgCandidates, "")

		var lastErr error
		for _, org := range orgCandidates {
			suite, lastErr = r.client.CreateSuite(ctx, &gi.Suite{Name: name, Folder: folderID, Organization: org})
			if lastErr == nil {
				break
			}
		}
		if suite == nil {
			resp.Diagnostics.AddError("Suite creation failed", fmt.Sprintf("tried %d organization candidate(s), last error: %v", len(orgCandidates), lastErr))
			return
		}
	}

	if err := r.pushSettings(ctx, suite.ID, &plan); err != nil {
		resp.Diagnostics.AddError("Suite settings sync failed", err.Error())
		return
	}

	if err := r.refresh(ctx, suite.ID, &plan); err != nil {
		resp.Diagnostics.AddError("Suite read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// pushSettings posts only the configured (non-null) settings to the suite.
func (r *SuiteResource) pushSettings(ctx context.Context, id string, plan *SuiteResourceModel) error {
	fields := plan.settingsModel.apiFields()
	fields["name"] = plan.Name.ValueString()
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		fields["description"] = plan.Description.ValueString()
	}
	if !plan.Schedule.IsNull() && !plan.Schedule.IsUnknown() {
		var sched scheduleModel
		if diags := plan.Schedule.As(ctx, &sched, basetypes.ObjectAsOptions{}); diags.HasError() {
			return fmt.Errorf("decode schedule plan")
		}
		entry := map[string]interface{}{}
		if !sched.Enabled.IsNull() && !sched.Enabled.IsUnknown() {
			entry["enabled"] = sched.Enabled.ValueBool()
		}
		if !sched.Interval.IsNull() && !sched.Interval.IsUnknown() {
			entry["interval"] = sched.Interval.ValueString()
		}
		if !sched.Time.IsNull() && !sched.Time.IsUnknown() {
			entry["time"] = sched.Time.ValueString()
		}
		fields["schedule"] = entry
	}
	return r.client.UpdateSuite(ctx, id, fields)
}

func (r *SuiteResource) refresh(ctx context.Context, id string, state *SuiteResourceModel) error {
	suite, err := r.client.GetSuite(ctx, id)
	if err != nil {
		return err
	}
	state.fromAPI(suite)
	return nil
}

func (r *SuiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SuiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.refresh(ctx, state.ID.ValueString(), &state)
	if gi.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Suite read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SuiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SuiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.pushSettings(ctx, plan.ID.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Suite update failed", err.Error())
		return
	}
	if err := r.refresh(ctx, plan.ID.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Suite read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SuiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SuiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSuite(ctx, state.ID.ValueString()); err != nil && !gi.NotFound(err) {
		resp.Diagnostics.AddError("Suite deletion failed", err.Error())
		return
	}
}

func (r *SuiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
