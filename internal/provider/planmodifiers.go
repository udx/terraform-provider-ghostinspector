package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// emptyStringIsNull collapses a configured empty string to null at plan
// time. The Ghost Inspector API treats absent and empty values identically,
// so "" would otherwise produce "inconsistent result after apply" when the
// read back maps the empty value to null.
func emptyStringIsNull() planmodifier.String {
	return emptyStringIsNullModifier{}
}

type emptyStringIsNullModifier struct{}

func (m emptyStringIsNullModifier) Description(_ context.Context) string {
	return "Treats an empty string as null."
}

func (m emptyStringIsNullModifier) MarkdownDescription(_ context.Context) string {
	return "Treats an empty string as null."
}

func (m emptyStringIsNullModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == "" {
		resp.PlanValue = types.StringNull()
	}
}
