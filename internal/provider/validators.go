package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// nonEmptyString rejects a configured empty string. The Ghost Inspector API
// treats absent and empty values identically, so an empty string would read
// back as null and fail apply with "inconsistent result after apply". A
// plan-time collapse to null is not possible: core rejects a planned value
// that differs from a configured one.
func nonEmptyString() validator.String {
	return nonEmptyStringValidator{}
}

type nonEmptyStringValidator struct{}

func (v nonEmptyStringValidator) Description(_ context.Context) string {
	return "Rejects an empty string."
}

func (v nonEmptyStringValidator) MarkdownDescription(_ context.Context) string {
	return "Rejects an empty string."
}

func (v nonEmptyStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Empty string not allowed",
			"The Ghost Inspector API treats empty and absent values identically, so an empty string would read back as null and fail apply. Omit the attribute to leave the API-side value unmanaged.",
		)
	}
}
