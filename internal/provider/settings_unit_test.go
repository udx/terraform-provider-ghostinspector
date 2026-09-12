package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

// The Ghost Inspector API discards geolocation on update and never returns it
// on read, so fromAPI must keep the configured value instead of nulling it
// (which Terraform reports as "Provider produced inconsistent result after
// apply").
func TestSuiteFromAPI_geolocationWriteOnly(t *testing.T) {
	geo := "40.7128,-74.006"

	// Configured value survives an API response without geolocation.
	m := SuiteResourceModel{}
	m.Geolocation = types.StringValue(geo)
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite"})
	if got := m.Geolocation.ValueString(); got != geo {
		t.Fatalf("configured geolocation lost: got %q, want %q", got, geo)
	}

	// Unknown (computed, not configured) resolves to null.
	m = SuiteResourceModel{}
	m.Geolocation = types.StringUnknown()
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite"})
	if !m.Geolocation.IsNull() {
		t.Fatalf("unknown geolocation should resolve to null, got %v", m.Geolocation)
	}

	// An API-returned value still wins.
	apiGeo := "51.5074,-0.1278"
	m = SuiteResourceModel{}
	m.Geolocation = types.StringValue(geo)
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite", Geolocation: &apiGeo})
	if got := m.Geolocation.ValueString(); got != apiGeo {
		t.Fatalf("API geolocation ignored: got %q, want %q", got, apiGeo)
	}
}

func TestTestFromAPI_geolocationWriteOnly(t *testing.T) {
	geo := "40.7128,-74.006"

	m := TestResourceModel{}
	m.Geolocation = types.StringValue(geo)
	m.fromAPI(&gi.Test{ID: "t1", Name: "test"})
	if got := m.Geolocation.ValueString(); got != geo {
		t.Fatalf("configured geolocation lost: got %q, want %q", got, geo)
	}

	m = TestResourceModel{}
	m.Geolocation = types.StringUnknown()
	m.fromAPI(&gi.Test{ID: "t1", Name: "test"})
	if !m.Geolocation.IsNull() {
		t.Fatalf("unknown geolocation should resolve to null, got %v", m.Geolocation)
	}

	apiGeo := "51.5074,-0.1278"
	m = TestResourceModel{}
	m.Geolocation = types.StringValue(geo)
	m.fromAPI(&gi.Test{ID: "t1", Name: "test", Geolocation: &apiGeo})
	if got := m.Geolocation.ValueString(); got != apiGeo {
		t.Fatalf("API geolocation ignored: got %q, want %q", got, apiGeo)
	}
}

// maxConcurrentTests is a plain managed suite setting: the API stores it and
// returns it on read (0 means unlimited), so fromAPI mirrors it.
func TestSuiteFromAPI_maxConcurrentTests(t *testing.T) {
	one := int64(1)
	m := SuiteResourceModel{}
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite", MaxConcurrentTests: &one})
	if got := m.MaxConcurrentTests.ValueInt64(); got != 1 {
		t.Fatalf("maxConcurrentTests not mapped: got %d, want 1", got)
	}

	// Zero is a real value (unlimited), not an absent one.
	zero := int64(0)
	m = SuiteResourceModel{}
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite", MaxConcurrentTests: &zero})
	if m.MaxConcurrentTests.IsNull() {
		t.Fatal("maxConcurrentTests=0 collapsed to null")
	}
	if got := m.MaxConcurrentTests.ValueInt64(); got != 0 {
		t.Fatalf("maxConcurrentTests=0 lost: got %d, want 0", got)
	}

	m = SuiteResourceModel{}
	m.MaxConcurrentTests = types.Int64Unknown()
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite"})
	if !m.MaxConcurrentTests.IsNull() {
		t.Fatalf("absent maxConcurrentTests should resolve to null, got %v", m.MaxConcurrentTests)
	}
}

// screenshotTarget / screenshotExclusions are stored by both the suite and
// test update APIs, so they flow through the shared settings model.
func TestSettingsAPIFields_screenshotFields(t *testing.T) {
	m := settingsModel{}
	m.ScreenshotTarget = types.StringValue(".hero")
	m.ScreenshotExclusions = types.StringValue(".ad, .carousel")
	fields := m.apiFields()
	if got := fields["screenshotTarget"]; got != ".hero" {
		t.Fatalf("screenshotTarget missing from API payload: %v", got)
	}
	if got := fields["screenshotExclusions"]; got != ".ad, .carousel" {
		t.Fatalf("screenshotExclusions missing from API payload: %v", got)
	}

	// Null means unmanaged: the keys are omitted from the payload entirely.
	m = settingsModel{}
	fields = m.apiFields()
	if _, ok := fields["screenshotTarget"]; ok {
		t.Fatalf("null screenshotTarget should be omitted, got %v", fields["screenshotTarget"])
	}
	if _, ok := fields["screenshotExclusions"]; ok {
		t.Fatalf("null screenshotExclusions should be omitted, got %v", fields["screenshotExclusions"])
	}

	// Empty string is an explicit clear and must be posted: the API stores ""
	// (whole-page capture), distinct from an absent key.
	m = settingsModel{}
	m.ScreenshotTarget = types.StringValue("")
	m.ScreenshotExclusions = types.StringValue("")
	fields = m.apiFields()
	if got, ok := fields["screenshotTarget"]; !ok || got != "" {
		t.Fatalf("empty screenshotTarget should be posted as an explicit clear, got %v (present=%v)", got, ok)
	}
	if got, ok := fields["screenshotExclusions"]; !ok || got != "" {
		t.Fatalf("empty screenshotExclusions should be posted as an explicit clear, got %v (present=%v)", got, ok)
	}
}

func TestSuiteFromAPI_screenshotFields(t *testing.T) {
	target := ".hero"
	exclusions := ".ad"
	m := SuiteResourceModel{}
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite", ScreenshotTarget: &target, ScreenshotExclusions: &exclusions})
	if got := m.ScreenshotTarget.ValueString(); got != target {
		t.Fatalf("screenshotTarget not mapped: got %q, want %q", got, target)
	}
	if got := m.ScreenshotExclusions.ValueString(); got != exclusions {
		t.Fatalf("screenshotExclusions not mapped: got %q, want %q", got, exclusions)
	}

	// An API-stored empty string (explicit clear) round-trips verbatim.
	empty := ""
	m = SuiteResourceModel{}
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite", ScreenshotTarget: &empty})
	if m.ScreenshotTarget.IsNull() {
		t.Fatal("empty screenshotTarget collapsed to null; explicit clear would not survive a read")
	}
	if got := m.ScreenshotTarget.ValueString(); got != "" {
		t.Fatalf("empty screenshotTarget changed: got %q", got)
	}

	m = SuiteResourceModel{}
	m.ScreenshotTarget = types.StringUnknown()
	m.fromAPI(&gi.Suite{ID: "s1", Name: "suite"})
	if !m.ScreenshotTarget.IsNull() {
		t.Fatalf("absent screenshotTarget should resolve to null, got %v", m.ScreenshotTarget)
	}
}

func TestTestFromAPI_screenshotFields(t *testing.T) {
	target := ".hero"
	exclusions := ".ad"
	m := TestResourceModel{}
	m.fromAPI(&gi.Test{ID: "t1", Name: "test", ScreenshotTarget: &target, ScreenshotExclusions: &exclusions})
	if got := m.ScreenshotTarget.ValueString(); got != target {
		t.Fatalf("screenshotTarget not mapped: got %q, want %q", got, target)
	}
	if got := m.ScreenshotExclusions.ValueString(); got != exclusions {
		t.Fatalf("screenshotExclusions not mapped: got %q, want %q", got, exclusions)
	}

	// An API-stored empty string (explicit clear) round-trips verbatim.
	empty := ""
	m = TestResourceModel{}
	m.fromAPI(&gi.Test{ID: "t1", Name: "test", ScreenshotTarget: &empty})
	if m.ScreenshotTarget.IsNull() {
		t.Fatal("empty screenshotTarget collapsed to null; explicit clear would not survive a read")
	}
	if got := m.ScreenshotTarget.ValueString(); got != "" {
		t.Fatalf("empty screenshotTarget changed: got %q", got)
	}

	m = TestResourceModel{}
	m.ScreenshotTarget = types.StringUnknown()
	m.fromAPI(&gi.Test{ID: "t1", Name: "test"})
	if !m.ScreenshotTarget.IsNull() {
		t.Fatalf("absent screenshotTarget should resolve to null, got %v", m.ScreenshotTarget)
	}
}

// maxConcurrentTests rejects negative values at plan time: the API stores a
// negative number as-is instead of rejecting it (verified live), which would
// surface as silent misbehavior at run time. 0 means unlimited.
func TestSuiteSchema_maxConcurrentTestsRejectsNegative(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&SuiteResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	attr, ok := schemaResp.Schema.Attributes["max_concurrent_tests"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("max_concurrent_tests is not an Int64Attribute")
	}
	if len(attr.Validators) == 0 {
		t.Fatal("max_concurrent_tests has no validators; a negative value would be posted to the API")
	}

	for _, tc := range []struct {
		value     int64
		wantError bool
	}{{-1, true}, {0, false}, {1, false}} {
		var errCount int
		for _, v := range attr.Validators {
			resp := &validator.Int64Response{}
			v.ValidateInt64(context.Background(), validator.Int64Request{
				Path:        path.Root("max_concurrent_tests"),
				ConfigValue: types.Int64Value(tc.value),
			}, resp)
			errCount += resp.Diagnostics.ErrorsCount()
		}
		if tc.wantError && errCount == 0 {
			t.Fatalf("max_concurrent_tests=%d should fail validation", tc.value)
		}
		if !tc.wantError && errCount != 0 {
			t.Fatalf("max_concurrent_tests=%d should pass validation", tc.value)
		}
	}
}
