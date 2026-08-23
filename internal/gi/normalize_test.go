package gi

import (
	"encoding/json"
	"testing"
)

func step(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	return m
}

func TestExtractTargetsShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"null target", `{"command":"click"}`, nil},
		{"empty string target", `{"command":"click","target":""}`, nil},
		{"empty array target", `{"command":"click","target":[]}`, nil},
		{"string target", `{"command":"click","target":".btn"}`, []string{".btn"}},
		{"single object array", `{"command":"click","target":[{"selector":".btn"}]}`, []string{".btn"}},
		{"single string array", `{"command":"click","target":[".btn"]}`, []string{".btn"}},
		{
			"multi selector array",
			`{"command":"click","target":[{"selector":".btn"},{"selector":"#alt"},{"selector":"xpath=//button"}]}`,
			[]string{".btn", "#alt", "xpath=//button"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanonicalizeStep(step(t, tc.raw))
			if len(got.Targets) != len(tc.want) {
				t.Fatalf("targets = %v, want %v", got.Targets, tc.want)
			}
			for i := range tc.want {
				if got.Targets[i] != tc.want[i] {
					t.Fatalf("targets = %v, want %v", got.Targets, tc.want)
				}
			}
		})
	}
}

func TestStepsEqualNormalizations(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			"string target equals selector-object array",
			`[{"command":"click","target":".btn"}]`,
			`[{"command":"click","target":[{"selector":".btn"}]}]`,
			true,
		},
		{
			"empty value equals absent value",
			`[{"command":"click","target":".btn","value":""}]`,
			`[{"command":"click","target":".btn"}]`,
			true,
		},
		{
			"empty notes equals absent notes",
			`[{"command":"click","target":".btn","notes":""}]`,
			`[{"command":"click","target":".btn"}]`,
			true,
		},
		{
			"absent optional equals false",
			`[{"command":"click","target":".btn"}]`,
			`[{"command":"click","target":".btn","optional":false}]`,
			true,
		},
		{
			"different selectors do not match",
			`[{"command":"click","target":".btn"}]`,
			`[{"command":"click","target":".other"}]`,
			false,
		},
		{
			"selector candidate order matters",
			`[{"command":"click","target":[{"selector":".a"},{"selector":".b"}]}]`,
			`[{"command":"click","target":[{"selector":".b"},{"selector":".a"}]}]`,
			false,
		},
		{
			"step order matters",
			`[{"command":"open","value":"/"},{"command":"click","target":".btn"}]`,
			`[{"command":"click","target":".btn"},{"command":"open","value":"/"}]`,
			false,
		},
		{
			"private flag is significant",
			`[{"command":"assign","target":"input","value":"{{pw}}","private":true}]`,
			`[{"command":"assign","target":"input","value":"{{pw}}"}]`,
			false,
		},
		{
			"condition object key order does not matter",
			`[{"command":"assertText","target":".t","value":"x","condition":{"statement":"return true;"}}]`,
			`[{"command":"assertText","target":".t","value":"x","condition":{"statement":"return true;"}}]`,
			true,
		},
		{
			"extra unknown fields are ignored",
			`[{"command":"click","target":".btn","extra":{"source":{"test":"abc"}},"_id":"1","sequence":3}]`,
			`[{"command":"click","target":".btn"}]`,
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a, b []map[string]interface{}
			if err := json.Unmarshal([]byte(tc.a), &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.b), &b); err != nil {
				t.Fatal(err)
			}
			if got := StepsEqual(a, b); got != tc.want {
				t.Fatalf("StepsEqual = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPIStepRoundTrip(t *testing.T) {
	raw := `[
		{"command":"open","value":"/login"},
		{"command":"click","target":[{"selector":".btn"},{"selector":"#alt"}],"optional":true},
		{"command":"assign","target":"input[name=\"pw\"]","value":"{{pw}}","private":true,"notes":"enter password"},
		{"command":"assertText","target":".title","value":"Hello","condition":{"statement":"return '{{env}}' == 'production';"}},
		{"command":"store","value":"quick","variableName":"animal"},
		{"command":"pause","value":"3000"}
	]`
	var in []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	canonical := CanonicalizeSteps(in)
	rendered := APISteps(canonical)
	if !StepsEqual(in, rendered) {
		dj, _ := json.Marshal(canonical)
		rj, _ := json.Marshal(rendered)
		t.Fatalf("round trip mismatch\ncanonical: %s\nrendered:  %s", dj, rj)
	}
}

func TestAPIStepTargetRendering(t *testing.T) {
	single := CanonicalStep{Command: "click", Targets: []string{".btn"}}
	got := single.APIStep()
	if v, ok := got["target"].(string); !ok || v != ".btn" {
		t.Fatalf("single target should render as string, got %v", got["target"])
	}

	multi := CanonicalStep{Command: "click", Targets: []string{".a", ".b"}}
	got = multi.APIStep()
	arr, ok := got["target"].([]map[string]interface{})
	if !ok || len(arr) != 2 || arr[0]["selector"] != ".a" || arr[1]["selector"] != ".b" {
		t.Fatalf("multi target should render as selector objects, got %v", got["target"])
	}
}
