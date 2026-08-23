package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var (
	_ resource.Resource                = &SuiteVariablesResource{}
	_ resource.ResourceWithImportState = &SuiteVariablesResource{}
)

// SuiteVariablesResource manages the complete variable set of one suite,
// matching the Ghost Inspector API's replace-all semantics. Private variable
// values are masked by the API on read; the provider carries the prior state
// value forward so private values do not produce perpetual diffs.
type SuiteVariablesResource struct {
	client *gi.Client
}

type variableModel struct {
	Name    types.String `tfsdk:"name"`
	Value   types.String `tfsdk:"value"`
	Private types.Bool   `tfsdk:"private"`
}

// SuiteVariablesResourceModel is the state model.
type SuiteVariablesResourceModel struct {
	ID        types.String `tfsdk:"id"`
	SuiteID   types.String `tfsdk:"suite_id"`
	Variables types.Set    `tfsdk:"variables"`
}

// NewSuiteVariablesResource creates the resource.
func NewSuiteVariablesResource() resource.Resource { return &SuiteVariablesResource{} }

func (r *SuiteVariablesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_suite_variables"
}

func variableAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":    types.StringType,
		"value":   types.StringType,
		"private": types.BoolType,
	}
}

func (r *SuiteVariablesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the complete set of variables on one Ghost Inspector suite. The API replaces the whole set on every update, so this resource owns every variable on the suite. Private values are write-only: the API masks them on read, and the provider keeps the configured value in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Resource ID (same as suite_id).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"suite_id": schema.StringAttribute{
				Required:    true,
				Description: "Suite whose variables are managed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"variables": schema.SetNestedAttribute{
				Required:    true,
				Description: "Complete variable set for the suite. Omitting a variable that exists on the suite removes it on the next apply.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Variable name, referenced in tests as {{name}}.",
						},
						"value": schema.StringAttribute{
							Optional:    true,
							Sensitive:   true,
							Description: "Variable value. May contain {{otherVariable}} references.",
						},
						"private": schema.BoolAttribute{
							Optional: true, Computed: true,
							Default:     booldefault.StaticBool(false),
							Description: "Mask the value in the Ghost Inspector UI and API.",
						},
					},
				},
			},
		},
	}
}

func (r *SuiteVariablesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func expandVariables(ctx context.Context, set types.Set) ([]variableModel, error) {
	vars := make([]variableModel, 0, len(set.Elements()))
	if set.IsNull() || set.IsUnknown() {
		return vars, nil
	}
	if diags := set.ElementsAs(ctx, &vars, false); diags.HasError() {
		return nil, fmt.Errorf("decode variables")
	}
	return vars, nil
}

func flattenVariables(vars []variableModel) types.Set {
	elems := make([]attr.Value, 0, len(vars))
	for _, v := range vars {
		elems = append(elems, types.ObjectValueMust(variableAttrTypes(), map[string]attr.Value{
			"name":    v.Name,
			"value":   v.Value,
			"private": v.Private,
		}))
	}
	return types.SetValueMust(types.ObjectType{AttrTypes: variableAttrTypes()}, elems)
}

func (r *SuiteVariablesResource) push(ctx context.Context, suiteID string, vars []variableModel) error {
	payload := make([]map[string]interface{}, 0, len(vars))
	for _, v := range vars {
		entry := map[string]interface{}{"name": v.Name.ValueString()}
		if !v.Value.IsNull() && !v.Value.IsUnknown() {
			entry["value"] = v.Value.ValueString()
		} else {
			entry["value"] = ""
		}
		if !v.Private.IsNull() && !v.Private.IsUnknown() && v.Private.ValueBool() {
			entry["private"] = true
		}
		payload = append(payload, entry)
	}
	return r.client.UpdateSuite(ctx, suiteID, map[string]interface{}{"variables": payload})
}

// masked reports whether an API-returned value for a private variable should
// be treated as masked (and the prior state value kept instead).
func masked(value string) bool {
	return value == "" || value == "***" || value == "•••" || value == "********"
}

// refresh reads the live variable set, carrying prior state values for
// private variables whose values come back masked.
func (r *SuiteVariablesResource) refresh(ctx context.Context, state *SuiteVariablesResourceModel) error {
	suite, err := r.client.GetSuite(ctx, state.SuiteID.ValueString())
	if err != nil {
		return err
	}

	prior := map[string]variableModel{}
	if priorVars, err := expandVariables(ctx, state.Variables); err == nil {
		for _, v := range priorVars {
			if !v.Name.IsNull() {
				prior[v.Name.ValueString()] = v
			}
		}
	}

	out := make([]variableModel, 0, len(suite.Variables))
	for _, v := range suite.Variables {
		entry := variableModel{
			Name:    types.StringValue(v.Name),
			Private: types.BoolValue(v.Private),
		}
		if v.Private && masked(v.Value) {
			if p, ok := prior[v.Name]; ok {
				entry.Value = p.Value
			} else {
				entry.Value = types.StringNull()
			}
		} else {
			entry.Value = types.StringValue(v.Value)
		}
		out = append(out, entry)
	}
	state.Variables = flattenVariables(out)
	state.ID = state.SuiteID
	return nil
}

func (r *SuiteVariablesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SuiteVariablesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vars, err := expandVariables(ctx, plan.Variables)
	if err != nil {
		resp.Diagnostics.AddError("Invalid variables", err.Error())
		return
	}
	if err := r.push(ctx, plan.SuiteID.ValueString(), vars); err != nil {
		resp.Diagnostics.AddError("Variable sync failed", err.Error())
		return
	}
	if err := r.refresh(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Suite read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SuiteVariablesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SuiteVariablesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.refresh(ctx, &state)
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

func (r *SuiteVariablesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SuiteVariablesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vars, err := expandVariables(ctx, plan.Variables)
	if err != nil {
		resp.Diagnostics.AddError("Invalid variables", err.Error())
		return
	}
	if err := r.push(ctx, plan.SuiteID.ValueString(), vars); err != nil {
		resp.Diagnostics.AddError("Variable sync failed", err.Error())
		return
	}
	if err := r.refresh(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Suite read failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SuiteVariablesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Removing this resource from configuration must not wipe the suite's
	// variables: omitting variables from a configuration means "unmanaged",
	// and a destroy that cleared the set would punish exactly that gesture.
	// The variable set is left on the suite; clear it via the UI or by
	// applying an empty variables list first.
	resp.Diagnostics.AddWarning(
		"Variables left in place",
		"The ghostinspector_suite_variables resource was removed from state only; the variable set still exists on the suite. Clear it in the Ghost Inspector UI or apply an empty variables list before removal if that is what you want.",
	)
}

func (r *SuiteVariablesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("suite_id"), req.ID)...)
}
