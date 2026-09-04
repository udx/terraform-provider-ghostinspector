package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var (
	_ resource.Resource                = &TestResource{}
	_ resource.ResourceWithImportState = &TestResource{}
)

// TestResource manages one Ghost Inspector test, including its steps.
//
// The Ghost Inspector API cannot update steps in place: a step change is
// applied as export -> overlay managed fields -> delete -> re-import, with a
// best-effort restore of the exported definition if the replacement import
// fails. The replacement receives a new test ID; historical results stay
// with the removed test. Any resource referencing this test's id attribute
// (for example an `execute` step in another test) sees the new value and is
// updated by the normal dependency graph.
//
// Creation adopts an existing test with the same name in the suite instead
// of creating a duplicate, which is how pre-existing suites are brought
// under management without churn.
type TestResource struct {
	client *gi.Client
}

type stepModel struct {
	Command      types.String         `tfsdk:"command"`
	Target       types.String         `tfsdk:"target"`
	Targets      types.List           `tfsdk:"targets"`
	Value        types.String         `tfsdk:"value"`
	Condition    jsontypes.Normalized `tfsdk:"condition"`
	VariableName types.String         `tfsdk:"variable_name"`
	Notes        types.String         `tfsdk:"notes"`
	Optional     types.Bool           `tfsdk:"optional"`
	Private      types.Bool           `tfsdk:"private"`
}

// TestResourceModel is the state model.
type TestResourceModel struct {
	ID         types.String `tfsdk:"id"`
	SuiteID    types.String `tfsdk:"suite_id"`
	Name       types.String `tfsdk:"name"`
	StartURL   types.String `tfsdk:"start_url"`
	ImportOnly types.Bool   `tfsdk:"import_only"`
	Steps      types.List   `tfsdk:"steps"`
	settingsModel
}

// NewTestResource creates the resource.
func NewTestResource() resource.Resource { return &TestResource{} }

func (r *TestResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test"
}

func stepAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"command":       types.StringType,
		"target":        types.StringType,
		"targets":       types.ListType{ElemType: types.StringType},
		"value":         types.StringType,
		"condition":     jsontypes.NormalizedType{},
		"variable_name": types.StringType,
		"notes":         types.StringType,
		"optional":      types.BoolType,
		"private":       types.BoolType,
	}
}

func (r *TestResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one Ghost Inspector test including its steps. Step changes replace the test (delete + re-import, new ID, history not carried over); metadata changes update in place. A null steps list leaves steps unmanaged. Null settings attributes leave the API-side values unmanaged (inherited from the suite).",
		Attributes: mergeAttrs(settingsAttributes(), map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Ghost Inspector test ID. Changes when steps are replaced.",
			},
			"suite_id": schema.StringAttribute{
				Required:    true,
				Description: "Suite the test belongs to. Moving a test replaces it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Test name. Used to match and adopt existing tests on create.",
			},
			"start_url": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Start URL, may contain {{variables}}. Null leaves the API value unmanaged.",
				PlanModifiers: []planmodifier.String{emptyStringIsNull(), stringplanmodifier.UseStateForUnknown()},
			},
			"import_only": schema.BoolAttribute{
				Optional: true, Computed: true,
				Description:   "Mark the test as an import-only shared module (hidden from normal suite listings).",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"steps": schema.ListNestedAttribute{
				Optional: true, Computed: true,
				Description: "Managed step list. When null, steps are not managed by this configuration. Changes replace the test with a new ID.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"command": schema.StringAttribute{
							Required:    true,
							Description: "Step command, for example open, click, assign, assertText, screenshot, execute.",
						},
						"target": schema.StringAttribute{
							Optional:    true,
							Description: "Single selector target. Conflicts with targets.",
						},
						"targets": schema.ListAttribute{
							Optional:    true,
							ElementType: types.StringType,
							Description: "Multiple selector candidates, in order. Conflicts with target.",
						},
						"value": schema.StringAttribute{
							Optional:    true,
							Description: "Step value (text to assign, assertion text, pause duration in ms, module test ID for execute, ...).",
						},
						"condition": schema.StringAttribute{
							Optional:    true,
							CustomType:  jsontypes.NormalizedType{},
							Description: "JSON condition object, for example {\"statement\": \"return true;\"}.",
						},
						"variable_name": schema.StringAttribute{
							Optional:    true,
							Description: "Variable name for commands that store a value.",
						},
						"notes": schema.StringAttribute{
							Optional:    true,
							Description: "Free-form step notes.",
						},
						"optional": schema.BoolAttribute{
							Optional: true, Computed: true,
							Description: "Allow the step to fail without failing the test.",
						},
						"private": schema.BoolAttribute{
							Optional: true, Computed: true,
							Description: "Mask the step value in the Ghost Inspector UI and results.",
						},
					},
				},
			},
		}),
	}
}

func (r *TestResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// expandSteps converts the planned step list into canonical form, validating
// the target/targets exclusivity.
func expandSteps(ctx context.Context, list types.List) ([]gi.CanonicalStep, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}
	var steps []stepModel
	if d := list.ElementsAs(ctx, &steps, false); d.HasError() {
		diags.Append(d...)
		return nil, diags
	}
	out := make([]gi.CanonicalStep, 0, len(steps))
	for i, s := range steps {
		var targets []string
		hasTarget := !s.Target.IsNull() && !s.Target.IsUnknown() && s.Target.ValueString() != ""
		hasTargets := !s.Targets.IsNull() && !s.Targets.IsUnknown() && len(s.Targets.Elements()) > 0
		if hasTarget && hasTargets {
			diags.AddError(
				"Invalid step",
				fmt.Sprintf("Step %d (%s) sets both target and targets; use target for one selector or targets for several candidates.", i, s.Command.ValueString()),
			)
			continue
		}
		if hasTarget {
			targets = []string{s.Target.ValueString()}
		}
		if hasTargets {
			var list []string
			if d := s.Targets.ElementsAs(ctx, &list, false); d.HasError() {
				diags.Append(d...)
				continue
			}
			targets = list
		}
		canonical := gi.CanonicalStep{
			Command: s.Command.ValueString(),
			Targets: targets,
		}
		if !s.Value.IsNull() && !s.Value.IsUnknown() && s.Value.ValueString() != "" {
			v := s.Value.ValueString()
			canonical.Value = &v
		}
		if !s.Condition.IsNull() && !s.Condition.IsUnknown() {
			canonical.Condition = json.RawMessage(s.Condition.ValueString())
		}
		if !s.VariableName.IsNull() && !s.VariableName.IsUnknown() && s.VariableName.ValueString() != "" {
			v := s.VariableName.ValueString()
			canonical.VariableName = &v
		}
		if !s.Notes.IsNull() && !s.Notes.IsUnknown() && s.Notes.ValueString() != "" {
			v := s.Notes.ValueString()
			canonical.Notes = &v
		}
		if !s.Optional.IsNull() && !s.Optional.IsUnknown() {
			canonical.Optional = s.Optional.ValueBool()
		}
		if !s.Private.IsNull() && !s.Private.IsUnknown() {
			canonical.Private = s.Private.ValueBool()
		}
		out = append(out, canonical)
	}
	return out, diags
}

// flattenSteps converts canonical steps (from the API) into the state list.
func flattenSteps(steps []gi.CanonicalStep) types.List {
	elems := make([]attr.Value, 0, len(steps))
	for _, s := range steps {
		obj := map[string]attr.Value{
			"command":       types.StringValue(s.Command),
			"target":        types.StringNull(),
			"targets":       types.ListNull(types.StringType),
			"value":         types.StringNull(),
			"condition":     jsontypes.NewNormalizedNull(),
			"variable_name": types.StringNull(),
			"notes":         types.StringNull(),
			"optional":      types.BoolValue(s.Optional),
			"private":       types.BoolValue(s.Private),
		}
		switch len(s.Targets) {
		case 0:
		case 1:
			obj["target"] = types.StringValue(s.Targets[0])
		default:
			vals := make([]attr.Value, 0, len(s.Targets))
			for _, t := range s.Targets {
				vals = append(vals, types.StringValue(t))
			}
			obj["targets"] = types.ListValueMust(types.StringType, vals)
		}
		if s.Value != nil {
			obj["value"] = types.StringValue(*s.Value)
		}
		if len(s.Condition) > 0 {
			obj["condition"] = jsontypes.NewNormalizedValue(string(s.Condition))
		}
		if s.VariableName != nil {
			obj["variable_name"] = types.StringValue(*s.VariableName)
		}
		if s.Notes != nil {
			obj["notes"] = types.StringValue(*s.Notes)
		}
		elems = append(elems, types.ObjectValueMust(stepAttrTypes(), obj))
	}
	return types.ListValueMust(types.ObjectType{AttrTypes: stepAttrTypes()}, elems)
}

// canonicalEqual compares desired canonical steps against raw API steps.
func canonicalEqual(desired []gi.CanonicalStep, currentRaw []map[string]interface{}) bool {
	current := gi.CanonicalizeSteps(currentRaw)
	dj, erra := json.Marshal(desired)
	cj, errb := json.Marshal(current)
	if erra != nil || errb != nil {
		return false
	}
	return string(dj) == string(cj)
}

// metadataFields renders the in-place-updatable fields for the plan.
func (m *TestResourceModel) metadataFields() map[string]interface{} {
	fields := m.settingsModel.apiFields()
	fields["name"] = m.Name.ValueString()
	if !m.StartURL.IsNull() && !m.StartURL.IsUnknown() {
		fields["startUrl"] = m.StartURL.ValueString()
	}
	if !m.ImportOnly.IsNull() && !m.ImportOnly.IsUnknown() {
		fields["importOnly"] = m.ImportOnly.ValueBool()
	}
	return fields
}

func (m *TestResourceModel) fromAPI(t *gi.Test) {
	m.ID = types.StringValue(t.ID)
	m.Name = types.StringValue(t.Name)
	m.SuiteID = stringOrNull(t.SuiteID())
	if t.StartURL != nil && *t.StartURL != "" {
		m.StartURL = types.StringValue(*t.StartURL)
	} else {
		m.StartURL = types.StringNull()
	}
	m.ImportOnly = boolOrNull(t.ImportOnly)
	m.Browser = stringOrNull(ptrStr(t.Browser))
	m.Region = stringOrNull(ptrStr(t.Region))
	m.UserAgent = stringOrNull(ptrStr(t.UserAgent))
	// The test update API silently discards geolocation, so it never comes
	// back on read. Keep the configured value in state (write-only).
	if g := ptrStr(t.Geolocation); g != "" {
		m.Geolocation = types.StringValue(g)
	} else if m.Geolocation.IsUnknown() {
		m.Geolocation = types.StringNull()
	}
	m.MaxWaitDelay = intOrNull(t.MaxWaitDelay)
	m.MaxAjaxDelay = intOrNull(t.MaxAjaxDelay)
	m.GlobalStepDelay = intOrNull(t.GlobalStepDelay)
	m.FinalDelay = intOrNull(t.FinalDelay)
	m.AutoRetry = boolOrNull(t.AutoRetry)
	m.ScreenshotCompareEnabled = boolOrNull(t.ScreenshotCompareEnabled)
	m.ScreenshotCompareThreshold = floatOrNull(t.ScreenshotCompareThreshold)
	m.FailOnJavaScriptError = boolOrNull(t.FailOnJavaScriptError)
	m.Steps = flattenSteps(gi.CanonicalizeSteps(t.Steps))
}

// replaceSteps performs the export -> overlay -> delete -> re-import cycle
// and returns the replacement test's ID. If the replacement import fails, a
// best-effort restore of the exported definition is attempted.
func (r *TestResource) replaceSteps(ctx context.Context, oldID string, plan *TestResourceModel, desired []gi.CanonicalStep) (string, error) {
	exported, err := r.client.ExportTest(ctx, oldID)
	if err != nil {
		return "", fmt.Errorf("export before replacement failed: %w", err)
	}

	doc := exported
	for k, v := range plan.metadataFields() {
		doc[k] = v
	}
	doc["steps"] = gi.APISteps(desired)

	if err := r.client.DeleteTest(ctx, oldID); err != nil {
		return "", fmt.Errorf("delete before re-import failed: %w", err)
	}

	replacement, err := r.client.ImportTest(ctx, plan.SuiteID.ValueString(), doc)
	if err != nil {
		restore, restoreErr := r.client.ImportTest(ctx, plan.SuiteID.ValueString(), exported)
		if restoreErr != nil {
			return "", fmt.Errorf("replacement import failed (%v) AND restore of the exported test also failed (%v); the test is gone, restore it from a backup", err, restoreErr)
		}
		return "", fmt.Errorf("replacement import failed (%v); the original definition was restored as test %s and keeps no link to this resource - re-import or re-adopt it", err, restore.ID)
	}
	return replacement.ID, nil
}

func (r *TestResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TestResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, diags := expandSteps(ctx, plan.Steps)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	stepsManaged := !plan.Steps.IsNull() && !plan.Steps.IsUnknown()
	suiteID := plan.SuiteID.ValueString()

	tests, err := r.client.ListSuiteTests(ctx, suiteID)
	if err != nil {
		resp.Diagnostics.AddError("Test lookup failed", err.Error())
		return
	}
	var existing *gi.Test
	for i := range tests {
		if tests[i].Name == plan.Name.ValueString() {
			existing = &tests[i]
			break
		}
	}

	if existing != nil {
		resp.Diagnostics.AddWarning(
			"Adopted existing test",
			fmt.Sprintf("A test named %q already existed in the suite (%s) and was adopted into state.", plan.Name.ValueString(), existing.ID),
		)
		plan.ID = types.StringValue(existing.ID)

		if err := r.client.UpdateTestMetadata(ctx, existing.ID, plan.metadataFields()); err != nil {
			resp.Diagnostics.AddError("Metadata sync failed", err.Error())
			return
		}
		if stepsManaged {
			current, err := r.client.GetTest(ctx, existing.ID)
			if err != nil {
				resp.Diagnostics.AddError("Test read failed", err.Error())
				return
			}
			if !canonicalEqual(desired, current.Steps) {
				newID, err := r.replaceSteps(ctx, existing.ID, &plan, desired)
				if err != nil {
					resp.Diagnostics.AddError("Step replacement failed", err.Error())
					return
				}
				plan.ID = types.StringValue(newID)
			}
		}
	} else {
		doc := plan.metadataFields()
		if stepsManaged {
			doc["steps"] = gi.APISteps(desired)
		} else {
			doc["steps"] = []map[string]interface{}{}
		}
		created, err := r.client.ImportTest(ctx, suiteID, doc)
		if err != nil {
			resp.Diagnostics.AddError("Test import failed", err.Error())
			return
		}
		plan.ID = types.StringValue(created.ID)
	}

	if err := r.refreshWithRetry(ctx, plan.ID.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Test read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TestResource) refresh(ctx context.Context, id string, state *TestResourceModel) error {
	test, err := r.client.GetTest(ctx, id)
	if err != nil {
		return err
	}
	state.fromAPI(test)
	return nil
}

// refreshWithRetry tolerates the brief read-after-write lag the API shows
// right after a delete + re-import cycle.
func (r *TestResource) refreshWithRetry(ctx context.Context, id string, state *TestResourceModel) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		if err = r.refresh(ctx, id, state); !gi.NotFound(err) {
			return err
		}
	}
	return err
}

func (r *TestResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TestResourceModel
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
		resp.Diagnostics.AddError("Test read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TestResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TestResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var prior TestResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, diags := expandSteps(ctx, plan.Steps)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	stepsManaged := !plan.Steps.IsNull() && !plan.Steps.IsUnknown()
	id := prior.ID.ValueString()
	plan.ID = prior.ID

	current, err := r.client.GetTest(ctx, id)
	if gi.NotFound(err) {
		resp.Diagnostics.AddError(
			"Test no longer exists",
			fmt.Sprintf("Test %s was deleted outside of this configuration. Run plan again to recreate it.", id),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Test read failed", err.Error())
		return
	}

	if err := r.client.UpdateTestMetadata(ctx, id, plan.metadataFields()); err != nil {
		resp.Diagnostics.AddError("Metadata update failed", err.Error())
		return
	}

	if stepsManaged && !canonicalEqual(desired, current.Steps) {
		newID, err := r.replaceSteps(ctx, id, &plan, desired)
		if err != nil {
			resp.Diagnostics.AddError("Step replacement failed", err.Error())
			return
		}
		plan.ID = types.StringValue(newID)
	}

	if err := r.refreshWithRetry(ctx, plan.ID.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Test read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TestResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TestResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTest(ctx, state.ID.ValueString()); err != nil && !gi.NotFound(err) {
		resp.Diagnostics.AddError("Test deletion failed", err.Error())
		return
	}
}

func (r *TestResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
