package provider

import (
	"testing"

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

	m = TestResourceModel{}
	m.ScreenshotTarget = types.StringUnknown()
	m.fromAPI(&gi.Test{ID: "t1", Name: "test"})
	if !m.ScreenshotTarget.IsNull() {
		t.Fatalf("absent screenshotTarget should resolve to null, got %v", m.ScreenshotTarget)
	}
}
