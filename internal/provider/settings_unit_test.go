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
}
