package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// nonNegativeInt64 rejects negative values. Ghost Inspector documents 0 as
// "unlimited" and positive values as the limit; the API stores a negative
// value as-is instead of rejecting it, which would surface as silent
// misbehavior at run time.
func nonNegativeInt64() validator.Int64 {
	return nonNegativeInt64Validator{}
}

type nonNegativeInt64Validator struct{}

func (v nonNegativeInt64Validator) Description(_ context.Context) string {
	return "Value must be zero or greater."
}

func (v nonNegativeInt64Validator) MarkdownDescription(_ context.Context) string {
	return "Value must be zero or greater."
}

func (v nonNegativeInt64Validator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueInt64() < 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Negative value not allowed",
			"The value must be 0 (unlimited) or a positive limit. Omit the attribute to leave the API-side value unmanaged.",
		)
	}
}
